package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/api"
	"github.com/entireio/cli/cmd/entire/cli/trail"
	"github.com/stretchr/testify/require"
)

func TestTrailCollectionsFollowOpaqueCursors(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"/api/v1/trails/gh/acme/widget", "/api/v1/repos/repo_example/trails"} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			for _, collection := range []string{"trails", "threads"} {
				t.Run(collection, func(t *testing.T) {
					t.Parallel()
					requests := 0
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests++
						wantPath := base
						if collection == "threads" {
							wantPath += "/7/threads"
						}
						if r.URL.Path != wantPath {
							t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
						}
						q := r.URL.Query()
						if q.Get("per_page") != "100" {
							t.Errorf("query = %s", r.URL.RawQuery)
						}
						w.Header().Set("Content-Type", "application/json")
						switch requests {
						case 1:
							if collection == "trails" && q.Get("status[eq]") != "open" {
								t.Errorf("initial filter = %s", r.URL.RawQuery)
							}
							_, _ = fmt.Fprint(w, `{"items":[{"id":"first","number":7,"status":"open","trail_id":"trail-example","kind":"discussion","message_count":2}],"next_cursor":"opaque+/=page","total_count":2}`)
						case 2:
							if q.Get("cursor") != "opaque+/=page" || len(q) != 2 {
								t.Errorf("continuation = %s", r.URL.RawQuery)
							}
							_, _ = fmt.Fprint(w, `{"items":[{"id":"second","number":8,"status":"open","trail_id":"trail-example","kind":"discussion","message_count":1}],"total_count":2}`)
						default:
							t.Errorf("unexpected page %d", requests)
						}
					}))
					defer srv.Close()
					client := api.NewClientWithBaseURL("tok", srv.URL)
					if collection == "threads" {
						items, err := fetchAllTrailThreads(t.Context(), client, base+"/7/threads")
						require.NoError(t, err)
						require.Len(t, items, 2)
						require.Equal(t, 2, items[0].MessageCount)
					} else {
						items, total, err := listTrailResources(t.Context(), client, base, []trail.Status{trail.StatusOpen}, "", 200)
						require.NoError(t, err)
						require.Len(t, items, 2)
						require.Equal(t, 2, total)
					}
					require.Equal(t, 2, requests)
				})
			}
		})
	}
}

func TestThreadJSONKeepsCLIKeys(t *testing.T) {
	t.Parallel()
	var detail api.TrailThreadDetailResponse
	require.NoError(t, json.Unmarshal([]byte(`{"thread":{"id":"th1","trail_id":"tr1","message_count":2,"created_at":"2026-09-01T00:00:00Z"},"messages":[{"id":"m1","created_at":"2026-09-01T00:00:00Z","replies":[{"id":"r1","created_at":"2026-09-01T00:01:00Z"}]}],"event_cursor":"42"}`), &detail))
	var out bytes.Buffer
	require.NoError(t, printTrailThreadDetail(&out, detail, true))
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.JSONEq(t, `"42"`, string(got["eventCursor"]))
	require.Contains(t, string(got["thread"]), `"trailId": "tr1"`)
	require.Contains(t, string(got["thread"]), `"messageCount": 2`)
	require.Contains(t, string(got["messages"]), `"createdAt": "2026-09-01T00:01:00Z"`)
}
