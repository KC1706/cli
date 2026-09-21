package cli

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/api"
	"github.com/entireio/cli/cmd/entire/cli/auth"
	"github.com/entireio/cli/internal/entireclient/clusterdiscovery"
	"github.com/entireio/cli/internal/entireclient/contexts"
	"github.com/entireio/cli/internal/entireclient/userdirs"
)

// The scheme gate has to run BEFORE credentials are resolved: discovery and the
// token refresh both dial the host, so checking afterwards would already have
// talked to it. NewAuthenticatedAPIClient relies on this one check rather than
// re-checking target.BaseURL afterwards, so this is what keeps that honest.
//
// Not parallel: it sets ENTIRE_API_BASE_URL and swaps the discovery seam.
func TestNewAuthenticatedAPIClient_RejectsInsecureOverrideBeforeResolving(t *testing.T) {
	if v, ok := os.LookupEnv(auth.EnvTokenVar); ok {
		os.Unsetenv(auth.EnvTokenVar)
		t.Cleanup(func() { os.Setenv(auth.EnvTokenVar, v) }) //nolint:usetesting // restoring a captured value; no t.Unsetenv equivalent
	}
	t.Setenv(userdirs.EnvConfigDir, t.TempDir())
	t.Setenv(userdirs.EnvCacheHome, t.TempDir())
	t.Setenv(api.BaseURLEnvVar, "http://data.invalid")
	t.Cleanup(auth.SetResolveContextForAPIForTest(t,
		func(context.Context, string, string, string, *http.Client, clusterdiscovery.DebugFunc) (*contexts.Context, error) {
			t.Fatal("discovery ran against an insecure override")
			return nil, errors.New("unreachable")
		}))

	if _, err := NewAuthenticatedAPIClient(t.Context(), false); !errors.Is(err, api.ErrInsecureHTTP) {
		t.Fatalf("error = %v, want ErrInsecureHTTP", err)
	}
}

// --insecure-http-auth is the documented opt-in, so the same override must get
// through it — otherwise local dev against an http data host is unreachable.
func TestNewAuthenticatedAPIClient_InsecureFlagAllowsHTTPOverride(t *testing.T) {
	if v, ok := os.LookupEnv(auth.EnvTokenVar); ok {
		os.Unsetenv(auth.EnvTokenVar)
		t.Cleanup(func() { os.Setenv(auth.EnvTokenVar, v) }) //nolint:usetesting // restoring a captured value; no t.Unsetenv equivalent
	}
	t.Setenv(userdirs.EnvConfigDir, t.TempDir())
	t.Setenv(userdirs.EnvCacheHome, t.TempDir())
	t.Setenv(api.BaseURLEnvVar, "http://data.invalid")
	discovered := false
	t.Cleanup(auth.SetResolveContextForAPIForTest(t,
		func(context.Context, string, string, string, *http.Client, clusterdiscovery.DebugFunc) (*contexts.Context, error) {
			discovered = true
			return nil, errors.New("no saved login")
		}))

	_, err := NewAuthenticatedAPIClient(t.Context(), true)
	if errors.Is(err, api.ErrInsecureHTTP) {
		t.Fatalf("error = %v, want the scheme gate bypassed under --insecure-http-auth", err)
	}
	if !discovered {
		t.Fatal("discovery did not run: the http override was rejected despite the opt-in")
	}
}
