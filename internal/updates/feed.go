package updates

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FeedDocument is the signed update channel payload: a canonical manifest
// plus its detached base64 ed25519 signature.
type FeedDocument struct {
	Manifest  Manifest `json:"manifest"`
	Signature string   `json:"signature"`
}

// DefaultFeedClient returns an HTTP client suited for feed polling from an
// appliance that may be offline: bounded total time, no cookie jar.
func DefaultFeedClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

// FetchFeed retrieves and decodes the feed document from url.
func FetchFeed(ctx context.Context, client *http.Client, url string) (FeedDocument, error) {
	if client == nil {
		client = DefaultFeedClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FeedDocument{}, fmt.Errorf("update feed request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return FeedDocument{}, fmt.Errorf("update feed unreachable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return FeedDocument{}, fmt.Errorf("update feed returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return FeedDocument{}, fmt.Errorf("update feed read failed: %w", err)
	}
	var document FeedDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return FeedDocument{}, fmt.Errorf("update feed payload is invalid: %w", err)
	}
	return document, nil
}

// VerifyFeedDocument checks the manifest structure and its ed25519 signature.
func VerifyFeedDocument(publicKey []byte, document FeedDocument) (Manifest, error) {
	var public ed25519.PublicKey
	if len(publicKey) == ed25519.PublicKeySize {
		public = ed25519.PublicKey(publicKey)
	} else {
		var err error
		public, err = ParsePublicKey(string(publicKey))
		if err != nil {
			return Manifest{}, err
		}
	}
	signature, err := ParseSignature(document.Signature)
	if err != nil {
		return Manifest{}, err
	}
	if err := VerifyManifest(public, document.Manifest, signature); err != nil {
		return Manifest{}, err
	}
	return document.Manifest, nil
}

// CompareVersions orders dotted version strings numerically where possible.
// Missing segments rank as zero; non-numeric segments compare lexically after
// numeric prefixes. Returns -1 when a is older, 0 when equal, 1 when newer.
func CompareVersions(a, b string) int {
	a = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a), "v"))
	b = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(b), "v"))
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for index := 0; index < len(as) || index < len(bs); index++ {
		result := compareSegment(segment(as, index), segment(bs, index))
		if result != 0 {
			return result
		}
	}
	return 0
}

func segment(parts []string, index int) string {
	if index < len(parts) {
		return parts[index]
	}
	return ""
}

func compareSegment(a, b string) int {
	if a == "" {
		a = "0"
	}
	if b == "" {
		b = "0"
	}
	aNumber, aErr := strconv.Atoi(a)
	bNumber, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		switch {
		case aNumber < bNumber:
			return -1
		case aNumber > bNumber:
			return 1
		default:
			return 0
		}
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
