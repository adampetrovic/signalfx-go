package signalfx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/adampetrovic/signalfx-go/sessiontoken"
)

// SessionTokenAPIURL is the base URL for interacting with org tokens.
const SessionTokenAPIURL = "/v2/session"

// CreateOrgToken creates a org token.
func (c *Client) CreateSessionToken(tokenRequest *sessiontoken.CreateTokenRequest) (*sessiontoken.Token, error) {
	payload, err := json.Marshal(tokenRequest)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest("POST", SessionTokenAPIURL, nil, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("Bad status %d: %s", resp.StatusCode, message)
	}

	sessionToken := &sessiontoken.Token{}

	err = json.NewDecoder(resp.Body).Decode(sessionToken)

	return sessionToken, err
}

// DeleteOrgToken deletes a token.
func (c *Client) DeleteSessionToken(token string) error {
	resp, err := c.doRequestWithToken("DELETE", SessionTokenAPIURL, nil, nil, token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		message, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Unexpected status code: %d: %s", resp.StatusCode, message)
	}

	return nil
}
