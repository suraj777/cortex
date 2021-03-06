package frontend

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/weaveworks/common/middleware"
	"github.com/weaveworks/common/user"
)

const seconds = 1e3 // 1e3 milliseconds per second.

func TestNextDayBoundary(t *testing.T) {
	for i, tc := range []struct {
		in, step, out int64
	}{
		{0, 1, millisecondPerDay - 1},
		{0, 15 * seconds, millisecondPerDay - 15*seconds},
		{1 * seconds, 15 * seconds, millisecondPerDay - (15-1)*seconds},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			require.Equal(t, tc.out, nextDayBoundary(tc.in, tc.step))
		})
	}
}

func TestSplitQuery(t *testing.T) {
	for i, tc := range []struct {
		input    *queryRangeRequest
		expected []*queryRangeRequest
	}{
		{
			input: &queryRangeRequest{
				start: 0,
				end:   60 * 60 * seconds,
				step:  15 * seconds,
				query: "foo",
			},
			expected: []*queryRangeRequest{
				{
					start: 0,
					end:   60 * 60 * seconds,
					step:  15 * seconds,
					query: "foo",
				},
			},
		},
		{
			input: &queryRangeRequest{
				start: 0,
				end:   24 * 3600 * seconds,
				step:  15 * seconds,
				query: "foo",
			},
			expected: []*queryRangeRequest{
				{
					start: 0,
					end:   24 * 3600 * seconds,
					step:  15 * seconds,
					query: "foo",
				},
			},
		},
		{
			input: &queryRangeRequest{
				start: 0,
				end:   2 * 24 * 3600 * seconds,
				step:  15 * seconds,
				query: "foo",
			},
			expected: []*queryRangeRequest{
				{
					start: 0,
					end:   (24 * 3600 * seconds) - (15 * seconds),
					step:  15 * seconds,
					query: "foo",
				},
				{
					start: 24 * 3600 * seconds,
					end:   2 * 24 * 3600 * seconds,
					step:  15 * seconds,
					query: "foo",
				},
			},
		},
		{
			input: &queryRangeRequest{
				start: 3 * 3600 * seconds,
				end:   3 * 24 * 3600 * seconds,
				step:  15 * seconds,
				query: "foo",
			},
			expected: []*queryRangeRequest{
				{
					start: 3 * 3600 * seconds,
					end:   (24 * 3600 * seconds) - (15 * seconds),
					step:  15 * seconds,
					query: "foo",
				},
				{
					start: 24 * 3600 * seconds,
					end:   (2 * 24 * 3600 * seconds) - (15 * seconds),
					step:  15 * seconds,
					query: "foo",
				},
				{
					start: 2 * 24 * 3600 * seconds,
					end:   3 * 24 * 3600 * seconds,
					step:  15 * seconds,
					query: "foo",
				},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			require.Equal(t, tc.expected, splitQuery(tc.input))
		})
	}
}

func TestSplitByDay(t *testing.T) {
	s := httptest.NewServer(
		middleware.AuthenticateUser.Wrap(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(responseBody))
			}),
		),
	)
	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(t, err)

	roundtripper := queryRangeRoundTripper{
		queryRangeMiddleware: splitByDay{
			next: queryRangeTerminator{
				next: singleHostRoundTripper{
					host: u.Host,
					next: http.DefaultTransport,
				},
			},
		},
	}

	mergedResponse, err := mergeAPIResponses([]*apiResponse{
		parsedResponse,
		parsedResponse,
	})
	require.NoError(t, err)

	mergedHTTPResponse, err := mergedResponse.toHTTPResponse()
	require.NoError(t, err)

	mergedHTTPResponseBody, err := ioutil.ReadAll(mergedHTTPResponse.Body)
	require.NoError(t, err)

	for i, tc := range []struct {
		path, expectedBody string
	}{
		{query, string(mergedHTTPResponseBody)},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			req, err := http.NewRequest("GET", tc.path, http.NoBody)
			require.NoError(t, err)

			ctx := user.InjectOrgID(context.Background(), "1")
			req = req.WithContext(ctx)

			resp, err := roundtripper.RoundTrip(req)
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)

			bs, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.expectedBody, string(bs))
		})
	}
}
