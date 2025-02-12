package template

// nolint: dupl

import (
	"context"
	"sync/atomic"

	"github.com/signalfx/golib/v3/datapoint"
	"github.com/signalfx/golib/v3/sfxclient"
)

//go:generate sh ./gen.sh

// InstancePreprocessor is used to filter out or otherwise change instances
// before being sent.  If the return value is false, the instance won't be
// sent.
type InstancePreprocessor func(*Instance) bool

// InstanceSender is what sends a slice of instances.  It should block until
// the instances have been sent, or an error has occurred.
type InstanceSender func(context.Context, []*Instance) error

const (
	DefaultInstanceMaxBuffered  = 10000
	DefaultInstanceMaxRequests  = 10
	DefaultInstanceMaxBatchSize = 1000
)

// InstanceWriter is an abstraction that accepts a bunch of instances, buffers
// them in a circular buffer and sends them out in concurrent batches.  This
// prioritizes newer Instances at the expense of older ones, which is generally
// desirable from a monitoring standpoint.
//
// You must call the non-blocking method Start on a created instance for it to
// do anything.
type InstanceWriter struct {
	// This must be provided by the user of this writer.
	InputChan chan []*Instance

	// PreprocessFunc can be used for filtering or modifying instances before
	// being sent.  If PreprocessFunc returns false, the instance will not be
	// sent. PreprocessFunc can be left nil, in which case all instances will
	// be sent.
	PreprocessFunc InstancePreprocessor

	// SendFunc must be provided as the writer is useless without it.  SendFunc
	// should synchronously process/send the Instances passed to it and not
	// return until they have been dealt with.  The slice passed to SendFunc
	// should not be used after the function returns, as its backing array
	// might get reused.
	SendFunc InstanceSender

	// OverwriteFunc can be set to a function that will be called
	// whenever an Add call to the underlying ring buffer results in the
	// overwriting of an unprocessed instance.
	OverwriteFunc func()

	// The maximum number of Instances that this writer will hold before
	// overwriting.  You must set this before calling Start.
	MaxBuffered int
	// The maximum number of concurrent calls to sendFunc that can be
	// active at a given instance.  You must set this before calling Start.
	MaxRequests int
	// The biggest batch of Instances the writer will emit to sendFunc at once.
	// You must set this before calling Start.
	MaxBatchSize int

	shutdownFlag  chan struct{}
	buff          *InstanceRingBuffer
	requestDoneCh chan int64

	// Holds up to MaxRequests slices that can be used to copy in Instance
	// pointers to avoid reusing the backing array of the ring buffer and
	// risking overwriting in the middle of sending.
	chunkSliceCache chan []*Instance

	requestsActive int64
	// Instances waiting to be sent but are blocked due to MaxRequests limit
	totalWaiting int64

	// Purely internal metrics.  If accessing any of these externally, use
	// atomic.LoadInt64!
	TotalReceived     int64
	TotalFilteredOut  int64
	TotalInFlight     int64
	TotalSent         int64
	TotalFailedToSend int64
	TotalOverwritten  int64
}

// WaitForShutdown will block until all of the elements inserted to the writer
// have been processed.
func (w *InstanceWriter) WaitForShutdown() {
	if w.shutdownFlag == nil {
		panic("should not wait for writer shutdown when not running")
	}

	<-w.shutdownFlag
}

// Returns a slice that has size len.  This reuses the backing array of the
// slices for all requests, so that only MaxRequests must be allocated for the
// lifetime of the writer.  Benchmark shows that this improves performance by
// ~5% and reduces allocations within the writer to almost zero.
func (w *InstanceWriter) getChunkSlice(size int) []*Instance {
	slice := <-w.chunkSliceCache

	// Nil out the elements above size in the slice so they will be GCed
	// quickly.  If you shorten a slice with the s[:n] trick, as below, without
	// niling out truncated elements, they won't be cleaned up.  If batches are
	// roughly the same size this will be minimal.
	for i := size; i < len(slice); i++ {
		slice[i] = nil
	}

	return slice[:size]
}

// Try and send the next batch in the buffer, if there are requests available.
func (w *InstanceWriter) tryToSendChunk(ctx context.Context) {
	totalUnprocessed := w.buff.UnprocessedCount()
	if w.requestsActive >= int64(w.MaxRequests) {
		w.totalWaiting = int64(totalUnprocessed)
		// The request done handler will notice that there are instances
		// waiting to be sent and will call this method again.
		return
	}

	chunk := w.buff.NextBatch(w.MaxBatchSize)

	count := int64(len(chunk))
	if count == 0 {
		return
	}

	atomic.AddInt64(&w.TotalInFlight, count)
	w.requestsActive++

	chunkCopy := w.getChunkSlice(len(chunk))
	// Make a copy of the slice in the buffer so that it is safe against
	// being overwritten by wrap around (NextBatch returns a slice against
	// the original backing array of the buffer).
	copy(chunkCopy, chunk)

	// Nil out existing elements in the buffer so they get GCd.
	for i := range chunk {
		chunk[i] = nil
	}

	go func() {
		err := w.SendFunc(ctx, chunkCopy)
		if err != nil {
			// Use atomic so that internal metrics method doesn't have to
			// run in the same goroutine.
			atomic.AddInt64(&w.TotalFailedToSend, count)
		} else {
			atomic.AddInt64(&w.TotalSent, count)
		}

		w.chunkSliceCache <- chunkCopy
		w.requestDoneCh <- count
	}()

	w.totalWaiting = int64(w.buff.UnprocessedCount())
}

func (w *InstanceWriter) processInput(ctx context.Context, insts []*Instance) {
	atomic.AddInt64(&w.TotalReceived, int64(len(insts)))
	for i := range insts {
		if w.PreprocessFunc != nil && !w.PreprocessFunc(insts[i]) {
			atomic.AddInt64(&w.TotalFilteredOut, 1)
			continue
		}

		if w.buff.Add(insts[i]) {
			atomic.AddInt64(&w.TotalOverwritten, 1)
			if w.OverwriteFunc != nil {
				w.OverwriteFunc()
			}
		}

		// Handle request done cleanup and try to send chunks if the buffer
		// gets full so that we can avoid overflowing the buffer on big input
		// slices where len(insts) > w.MaxBuffered.
		select {
		case count := <-w.requestDoneCh:
			w.handleRequestDone(ctx, count)
		default:
			// If there isn't any request done then continue on
		}

		if w.buff.UnprocessedCount() >= w.MaxBatchSize {
			w.tryToSendChunk(ctx)
		}
	}

}

// Start the writer processing loop
func (w *InstanceWriter) Start(ctx context.Context) {
	// Initialize the shutdownFlag in the same goroutine as the one calling
	// start to avoid data races when calling WaitForShutdown.
	w.shutdownFlag = make(chan struct{})
	go func() {
		w.run(ctx)
		close(w.shutdownFlag)
	}()
}

func (w *InstanceWriter) handleRequestDone(ctx context.Context, count int64) {
	w.requestsActive--
	atomic.AddInt64(&w.TotalInFlight, -count)

	if w.totalWaiting > 0 {
		w.tryToSendChunk(ctx)
	}
}

// run waits for Instances to come in on the provided channel and gives them to
// sendFunc in batches.  This function blocks until the provided context is
// canceled.
//nolint: dupl
func (w *InstanceWriter) run(ctx context.Context) {
	if w.MaxBuffered == 0 {
		w.MaxBuffered = DefaultInstanceMaxBuffered
	}
	if w.MaxRequests == 0 {
		w.MaxRequests = DefaultInstanceMaxRequests
	}
	if w.MaxBatchSize == 0 {
		w.MaxBatchSize = DefaultInstanceMaxBatchSize
	}

	w.buff = NewInstanceRingBuffer(w.MaxBuffered)

	// Make the slice copy cache and prime it with preallocated slices
	w.chunkSliceCache = make(chan []*Instance, w.MaxRequests)
	for i := 0; i < w.MaxRequests; i++ {
		w.chunkSliceCache <- make([]*Instance, 0, w.MaxBatchSize)
	}

	w.requestDoneCh = make(chan int64, w.MaxRequests)

	waitForRequests := func() {
		for w.requestsActive > 0 {
			count := <-w.requestDoneCh
			w.handleRequestDone(ctx, count)
		}
	}

	drainInput := func() {
		defer waitForRequests()
		defer w.tryToSendChunk(ctx)
		for {
			select {
			case insts := <-w.InputChan:
				w.processInput(ctx, insts)
			default:
				return
			}
		}
	}

	// The main loop.  The basic technique is to pull as many Instances from
	// the input channel as possible until the channel is exhausted, at which
	// point the Instances are attempted to be sent.  All of the request
	// finalization is also handled here so that everything is done within a
	// single goroutine and does not require explicit locking.
	for {
		select {
		case <-ctx.Done():
			drainInput()
			return

		case insts := <-w.InputChan:
			w.processInput(ctx, insts)

		case count := <-w.requestDoneCh:
			w.handleRequestDone(ctx, count)

		default:
			// The input chan is exhaused, try to send whatever was there.
			w.tryToSendChunk(ctx)

			// Duplicate the cases from above to avoid hot looping and using
			// unnecessary CPU.
			select {
			case <-ctx.Done():
				drainInput()
				return

			case count := <-w.requestDoneCh:
				w.handleRequestDone(ctx, count)

			case insts := <-w.InputChan:
				w.processInput(ctx, insts)
			}
		}
	}
}

// InternalMetrics about the instance writer
func (w *InstanceWriter) InternalMetrics(prefix string) []*datapoint.Datapoint {
	return []*datapoint.Datapoint{
		sfxclient.CumulativeP(prefix+"instances_sent", nil, &w.TotalSent),
		sfxclient.CumulativeP(prefix+"instances_failed", nil, &w.TotalFailedToSend),
		sfxclient.CumulativeP(prefix+"instances_filtered", nil, &w.TotalFilteredOut),
		sfxclient.CumulativeP(prefix+"instances_received", nil, &w.TotalReceived),
		sfxclient.CumulativeP(prefix+"instances_overwritten", nil, &w.TotalOverwritten),
		sfxclient.Gauge(prefix+"instances_buffered", nil, int64(w.buff.UnprocessedCount())),
		sfxclient.Gauge(prefix+"instances_max_buffered", nil, int64(w.buff.Size())),
		sfxclient.Gauge(prefix+"instances_in_flight", nil, atomic.LoadInt64(&w.TotalInFlight)),
		sfxclient.Gauge(prefix+"instances_waiting", nil, atomic.LoadInt64(&w.totalWaiting)),
		sfxclient.Gauge(prefix+"instance_requests_active", nil, atomic.LoadInt64(&w.requestsActive)),
	}
}
