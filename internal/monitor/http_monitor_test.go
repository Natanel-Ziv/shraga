package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestHttpMonitor_BeforeSave(t *testing.T) {
	hm := &HttpMonitor{
		Address:          "https://example.com",
		ValidStatusCodes: []int{200, 301},
		ReqHeaders:       map[string]string{"Authorization": "Bearer token"},
		ReqTimeout:       10 * time.Second,
	}

	mockDB := &gorm.DB{}
	err := hm.BeforeSave(mockDB)
	assert.NoError(t, err)
	assert.NotEmpty(t, hm.ValidStatusCodesJSON)
	assert.NotEmpty(t, hm.ReqHeadersJSON)
	assert.Equal(t, int64(10*time.Second), hm.ReqTimeoutInt)
}

func TestHttpMonitor_AfterFind(t *testing.T) {
	hm := &HttpMonitor{
		ValidStatusCodesJSON: `[200, 301]`,
		ReqHeadersJSON:       `{"Authorization": "Bearer token"}`,
		ReqTimeoutInt:        int64(10 * time.Second),
	}

	mockDB := &gorm.DB{}
	err := hm.AfterFind(mockDB)
	assert.NoError(t, err)
	assert.Equal(t, []int{200, 301}, hm.ValidStatusCodes)
	assert.Equal(t, map[string]string{"Authorization": "Bearer token"}, hm.ReqHeaders)
	assert.Equal(t, 10*time.Second, hm.ReqTimeout)
}

func TestHttpMonitor_Monitor_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Expected response"))
	}))
	defer ts.Close()

	hm := &HttpMonitor{
		Address:             ts.URL,
		RequestMethod:       http.MethodGet,
		ValidStatusCodes:    []int{200},
		ShouldCheckResponse: true,
		ExpectedResponse:    "Expected response",
		ReqTimeout:          5 * time.Second,
	}

	ctx := context.Background()
	response := hm.Monitor(ctx)

	assert.NotNil(t, response)
	assert.Equal(t, ResultUp, response.GetBaseMonitorResponse().Result)
	assert.True(t, response.(*HttpResponse).StatusCodeValid)
	assert.Equal(t, "", response.GetBaseMonitorResponse().ErrorMsg)
}

func TestHttpMonitor_Monitor_Failure_StatusCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	hm := &HttpMonitor{
		Address:          ts.URL,
		RequestMethod:    http.MethodGet,
		ValidStatusCodes: []int{200},
		ReqTimeout:       5 * time.Second,
	}

	ctx := context.Background()
	response := hm.Monitor(ctx)

	assert.NotNil(t, response)
	assert.Equal(t, ResultDown, response.GetBaseMonitorResponse().Result)
	assert.False(t, response.(*HttpResponse).StatusCodeValid)
}

func TestHttpMonitor_Monitor_Failure_ResponseBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Unexpected response"))
	}))
	defer ts.Close()

	hm := &HttpMonitor{
		Address:             ts.URL,
		RequestMethod:       http.MethodGet,
		ValidStatusCodes:    []int{200},
		ShouldCheckResponse: true,
		ExpectedResponse:    "Expected response",
		ReqTimeout:          5 * time.Second,
	}

	ctx := context.Background()
	response := hm.Monitor(ctx)

	assert.NotNil(t, response)
	assert.Equal(t, ResultDown, response.GetBaseMonitorResponse().Result)
	assert.Equal(t, "response is not as expected: Unexpected response", response.GetBaseMonitorResponse().ErrorMsg)
}

func TestHttpMonitor_Monitor_Failure_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(6 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	hm := &HttpMonitor{
		Address:          ts.URL,
		RequestMethod:    http.MethodGet,
		ValidStatusCodes: []int{200},
		ReqTimeout:       2 * time.Second,
	}

	ctx := context.Background()
	response := hm.Monitor(ctx)

	assert.NotNil(t, response)
	assert.Equal(t, ResultDown, response.GetBaseMonitorResponse().Result)
	assert.Contains(t, response.GetBaseMonitorResponse().ErrorMsg, "context deadline exceeded")
}

func TestHttpMonitor_checkSSL_Valid(t *testing.T) {
	hm := &HttpMonitor{
		Address: "https://google.com",
	}

	sslDetails := hm.checkSSL()
	assert.True(t, sslDetails.Valid)
	assert.True(t, sslDetails.Expiry.After(time.Now()))
}

func TestHttpMonitor_checkSSL_Invalid(t *testing.T) {
	hm := &HttpMonitor{
		Address: "https://invalid-url",
	}

	sslDetails := hm.checkSSL()
	assert.False(t, sslDetails.Valid)
}

func TestHttpMonitor_BeforeSave_TimeoutValidation(t *testing.T) {
	hm := &HttpMonitor{
		ReqTimeout: 10 * time.Minute,
	}

	mockDB := &gorm.DB{}
	err := hm.BeforeSave(mockDB)
	assert.NoError(t, err)
	assert.Equal(t, int64(maxHttpClientTimeout), hm.ReqTimeoutInt)
}

func TestHttpMonitor_AfterFind_TimeoutValidation(t *testing.T) {
	hm := &HttpMonitor{
		ReqTimeoutInt: int64(10 * time.Minute),
	}

	mockDB := &gorm.DB{}
	err := hm.AfterFind(mockDB)
	assert.NoError(t, err)
	assert.Equal(t, maxHttpClientTimeout, hm.ReqTimeout)
}
