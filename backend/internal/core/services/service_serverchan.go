package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// serverChanBaseURL is the base URL of the Server酱 (sct.ftqq.com) push API.
// It is a package-level variable so tests can point the client at a mock
// server.
var serverChanBaseURL = "https://sctapi.ftqq.com"

const (
	// serverChanTimeout bounds a single push request.
	serverChanTimeout = 10 * time.Second
	// serverChanMaxResponseBytes caps the API response body we are willing to
	// read; the real payload is a tiny JSON object.
	serverChanMaxResponseBytes = 64 * 1024
)

// serverChanResponse is the subset of the Server酱 API response we care
// about; code == 0 means the push was accepted.
type serverChanResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SendServerChan pushes a message through the Server酱 API:
// POST {base}/<sendKey>.send with form fields "title" and "desp". A response
// whose JSON "code" is not 0 is returned as an error carrying the API's
// message. It deliberately uses http.DefaultClient, which has the SSRF
// redirect guard installed at startup (internal/sys/validate).
func SendServerChan(ctx context.Context, sendKey, title, desp string) error {
	if sendKey == "" {
		return errors.New("serverchan sendkey is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, serverChanTimeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/%s.send", serverChanBaseURL, url.PathEscape(sendKey))
	form := url.Values{"title": {title}, "desp": {desp}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to build serverchan request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send serverchan request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Err(err).Msg("failed to close serverchan response body")
		}
	}()

	var result serverChanResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, serverChanMaxResponseBytes)).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode serverchan response: %w", err)
	}

	if result.Code != 0 {
		return fmt.Errorf("serverchan push failed (code %d): %s", result.Code, result.Message)
	}

	return nil
}
