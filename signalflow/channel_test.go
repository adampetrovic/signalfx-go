package signalflow

import (
	"context"
	"testing"

	"github.com/adampetrovic/signalfx-go/idtool"
	"github.com/adampetrovic/signalfx-go/signalflow/messages"
	"github.com/stretchr/testify/require"
)

func TestChannel(t *testing.T) {
	ch := newChannel(context.Background(), "test-ch")

	msg := messages.MetadataMessage{
		TSID: idtool.ID(4000),
	}
	go ch.AcceptMessage(&msg)

	require.Equal(t, &msg, (<-ch.Messages()).(*messages.MetadataMessage))
}
