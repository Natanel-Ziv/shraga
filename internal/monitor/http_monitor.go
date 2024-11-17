package monitor

import (
	"context"
	"crypto/tls"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"shraga/internal/logging"
	"strings"
	"time"

	"github.com/samber/lo"
	"gorm.io/gorm"
)

const (
	defaultHttpClientTimeout = 30 * time.Second
	maxHttpClientTimeout     = 5 * time.Minute
	minHttpClientTimeout     = 1 * time.Second
)

type HttpResponse struct {
	BaseMonitorResponse
	SslResp         SSLDetails
	Latency         int64
	DataValid       bool
	StatusCodeValid bool
}

// SSLDetails stores SSL-specific information
type SSLDetails struct {
	Valid  bool
	Expiry time.Time
}

// Valuer and Scanner implementation for SSLDetails
func (s SSLDetails) Value() (driver.Value, error) {
	return json.Marshal(s)
}

func (s *SSLDetails) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal SSLDetails value: %v", value)
	}

	return json.Unmarshal(bytes, s)
}

func (hr *HttpResponse) GetBaseMonitorResponse() *BaseMonitorResponse {
	return &hr.BaseMonitorResponse
}

type HttpMonitor struct {
	BaseMonitor
	Address               string
	ValidStatusCodes      []int  `gorm:"-"`
	ValidStatusCodesJSON  string `json:"-"`
	ShouldWarnOnSSLExpiry bool
	ShouldCheckSSL        bool
	ExpectedResponse      string
	ShouldCheckResponse   bool
	ReqBody               string
	ReqContentType        string
	ReqHeaders            map[string]string `gorm:"-"`
	ReqHeadersJSON        string
	RequestMethod         string
	ReqTimeoutInt         int64         `gorm:"column:req_timeout"`
	ReqTimeout            time.Duration `gorm:"-"`
}

func (hm *HttpMonitor) BeforeSave(tx *gorm.DB) (err error) {
	err = hm.BaseMonitor.BeforeSave(tx)
	if err != nil {
		return
	}

	// Serialize ValidStatusCodes to JSON
	if hm.ValidStatusCodes != nil {
		validCodesJSON, err := json.Marshal(hm.ValidStatusCodes)
		if err != nil {
			return err
		}
		hm.ValidStatusCodesJSON = string(validCodesJSON)
	}

	var headersJSON []byte
	if hm.ReqHeaders != nil {
		headersJSON, err = json.Marshal(hm.ReqHeaders)
		if err != nil {
			return
		}
		hm.ReqHeadersJSON = string(headersJSON)
	}

	if hm.ReqTimeout == 0 {
		hm.ReqTimeout = defaultHttpClientTimeout
	} else if hm.ReqTimeout > maxHttpClientTimeout {
		hm.ReqTimeout = maxHttpClientTimeout
	} else if hm.ReqTimeout < minHttpClientTimeout {
		hm.ReqTimeout = minHttpClientTimeout
	}
	hm.ReqTimeoutInt = int64(hm.ReqTimeout)

	return nil
}

func (hm *HttpMonitor) AfterFind(tx *gorm.DB) (err error) {
	err = hm.BaseMonitor.AfterFind(tx)
	if err != nil {
		return
	}

	// Deserialize ValidStatusCodes from JSON
	if hm.ValidStatusCodesJSON != "" {
		var validCodes []int
		if err := json.Unmarshal([]byte(hm.ValidStatusCodesJSON), &validCodes); err != nil {
			return err
		}
		hm.ValidStatusCodes = validCodes
	}

	if hm.ReqHeadersJSON != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(hm.ReqHeadersJSON), &headers); err != nil {
			return err
		}
		hm.ReqHeaders = headers
	}

	hm.ReqTimeout = time.Duration(hm.ReqTimeoutInt)
	if hm.ReqTimeout > maxHttpClientTimeout {
		hm.ReqTimeout = maxHttpClientTimeout
	} else if hm.ReqTimeout < minHttpClientTimeout {
		hm.ReqTimeout = minHttpClientTimeout
	}

	return nil
}

func (hm *HttpMonitor) Monitor(ctx context.Context) MonitorResponser {
	logging.Logger.Sugar().Infof("Start monitoring: %d", hm.ID)

	var monitorResult = &HttpResponse{
		BaseMonitorResponse: BaseMonitorResponse{
			MonitorID:    hm.ID,
			Result:       ResultDown,
			ResponseTime: now(),
		},
		SslResp: SSLDetails{},
	}

	var body io.Reader
	if len(hm.ReqBody) > 0 {
		body = strings.NewReader(hm.ReqBody)
	}

	req, err := http.NewRequestWithContext(ctx, hm.RequestMethod, hm.Address, body)
	if err != nil {
		monitorResult.ErrorMsg = err.Error()
		return monitorResult
	}

	// Set Content-Type if request body is provided
	if hm.ReqBody != "" && hm.ReqContentType != "" {
		req.Header.Set("Content-Type", hm.ReqContentType)
	}

	// Add custom headers
	for key, value := range hm.ReqHeaders {
		req.Header.Set(key, value)
	}

	if hm.ShouldCheckSSL || hm.ShouldWarnOnSSLExpiry {
		monitorResult.SslResp = hm.checkSSL()
	}

	client := &http.Client{Timeout: time.Duration(hm.ReqTimeout)}

	startTime := now()
	resp, err := client.Do(req)
	if err != nil {
		monitorResult.ErrorMsg = err.Error()
		return monitorResult
	}

	monitorResult.Latency = time.Since(startTime).Milliseconds()
	monitorResult.StatusCodeValid = lo.Contains(hm.ValidStatusCodes, resp.StatusCode)
	if !monitorResult.StatusCodeValid {
		monitorResult.Result = ResultDown
		return monitorResult
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logging.Logger.Sugar().Warn("Error closing response body", closeErr)
		}
	}()

	if hm.ShouldCheckResponse {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			monitorResult.ErrorMsg = err.Error()
			return monitorResult
		}

		gotResp := string(respBody)
		if gotResp != hm.ExpectedResponse {
			monitorResult.ErrorMsg = fmt.Sprintf("response is not as expected: %s", gotResp)
			return monitorResult
		}
	}

	if hm.ShouldWarnOnSSLExpiry && monitorResult.SslResp.Expiry.Sub(now()) < (30*24*time.Hour) {
		monitorResult.Result = ResultWarn
	} else {
		monitorResult.Result = ResultUp
	}

	return monitorResult
}

// checkSSL validates the SSL certificate and fetches its expiry date.
func (hm *HttpMonitor) checkSSL() SSLDetails {
	sslDetails := SSLDetails{}

	// Parse the URL to extract the hostname
	parsedURL, err := url.Parse(hm.Address)
	if err != nil {
		logging.Logger.Sugar().Errorf("Failed to parse URL: %v", err)
		sslDetails.Valid = false
		return sslDetails
	}

	// Ensure the URL has a valid hostname and scheme
	hostname := parsedURL.Host
	if !strings.Contains(hostname, ":") {
		hostname += ":443" // Add the default port if it's not already present
	}

	conn, err := tls.Dial("tcp", hostname, &tls.Config{})
	if err != nil {
		logging.Logger.Sugar().Errorf("Failed to establish SSL connection: %v", err)
		sslDetails.Valid = false
		return sslDetails
	}
	defer conn.Close()

	// Retrieve the certificate chain
	cert := conn.ConnectionState().PeerCertificates[0]
	sslDetails.Valid = true
	sslDetails.Expiry = cert.NotAfter

	return sslDetails
}

func (hm *HttpMonitor) IsEnabled() bool {
	return hm.Enabled
}

func (hm *HttpMonitor) GetType() MonitorType {
	return hm.Type
}
