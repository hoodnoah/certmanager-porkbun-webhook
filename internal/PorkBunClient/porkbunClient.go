package porkbunclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

const (
	// PorkBun API DNS record creation URL
	createURL = "https://api.porkbun.com/api/json/v3/dns/create/%s" // append domain name

	// PorkBun API DNS record deletion URL
	deleteURL = "https://api.porkbun.com/api/json/v3/dns/deleteByNameType/%s/TXT/%s" // append domain name, subdomain
)

type PorkBunClient struct {
	logger     logr.Logger
	httpClient http.Client
	APIKey     string
	SecretKey  string
}

type dnsCreateArgs struct {
	APIKey    string `json:"apikey"`
	SecretKey string `json:"secretapikey"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	TTL       string `json:"ttl"`
}

type dnsCreateResponse struct {
	Status string      `json:"status"`
	ID     json.Number `json:"id"`
}

type dnsDeleteArgs struct {
	APIKey    string `json:"apikey"`
	SecretKey string `json:"secretapikey"`
}

type dnsDeleteResponse struct {
	Status string `json:"status"`
}

func newDNSCreateArgs(apiKey string, secretKey string, subdomain string, content string) *dnsCreateArgs {
	return &dnsCreateArgs{
		APIKey:    apiKey,
		SecretKey: secretKey,
		Name:      subdomain,
		Type:      "TXT",
		Content:   content,
		TTL:       "600",
	}
}

// constructor for porkbun client
func NewPorkBunClient(logger logr.Logger, apiKey string, secretKey string) *PorkBunClient {
	return &PorkBunClient{
		logger:     logger,
		httpClient: http.Client{Timeout: 10 * time.Second},
		APIKey:     apiKey,
		SecretKey:  secretKey,
	}
}

// create a new DNS record, returning the ID of the created record
func (c *PorkBunClient) CreateDNSRecord(domain string, content string, subdomain string) error {
	c.logger.Info("Creating DNS record", "domain", domain, "subdomain", subdomain, "content", content)

	url := fmt.Sprintf(createURL, domain)
	c.logger.Info("DNS record create URL", "url", url)

	// create request body
	args := newDNSCreateArgs(c.APIKey, c.SecretKey, subdomain, content)
	c.logger.Info("DNS record create args", "args", args)

	body, err := json.Marshal(args)
	if err != nil {
		return err
	}
	c.logger.Info("DNS record create body", "body", string(body))

	// create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		c.logger.Error(err, "Failed to create DNS record create request")
		return err
	}

	// set request headers
	req.Header.Set("Content-Type", "application/json")

	// send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error(err, "Failed to send DNS record create request")
		return err
	}
	defer resp.Body.Close()

	// check response status code
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		c.logger.Info("Unexpected status code from DNS record create response", "status", resp.Status, "response", string(responseBody))
		return fmt.Errorf("unexpected status code: %s, response: %s", resp.Status, string(responseBody))
	}

	// parse response
	var response dnsCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		c.logger.Error(err, "Failed to decode DNS record create response")
		return err
	}

	if response.Status != "SUCCESS" {
		c.logger.Info("Unexpected DNS record create response status", "status", response.Status)
		return fmt.Errorf("unexpected response status: %s", response.Status)
	}

	return nil
}

// delete an existing DNS record by domain and id
func (c *PorkBunClient) DeleteDNSRecord(domain string, subdomain string) error {
	url := fmt.Sprintf(deleteURL, domain, subdomain)

	deleteArgs := dnsDeleteArgs{
		APIKey:    c.APIKey,
		SecretKey: c.SecretKey,
	}

	// create request body
	body, err := json.Marshal(deleteArgs)
	if err != nil {
		c.logger.Error(err, "Failed to marshal DNS delete request body")
		return nil
	}

	// create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))

	// set request headers
	req.Header.Set("Content-Type", "application/json")

	// send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error(err, "Failed to send DNS delete request")
		return err
	}

	// check response status code
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		c.logger.Info("Unexpected status code from DNS delete response", "status", resp.Status, "response", string(responseBody))
		return fmt.Errorf("unexpected status code: %s, response: %s", resp.Status, string(responseBody))
	}

	// parse response
	var response dnsDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		c.logger.Error(err, "Failed to decode DNS delete response")
		return err
	}

	if response.Status != "SUCCESS" {
		c.logger.Info("Unexpected DNS delete response status", "status", response.Status)
		return fmt.Errorf("unexpected response status: %s", response.Status)
	}

	return nil
}
