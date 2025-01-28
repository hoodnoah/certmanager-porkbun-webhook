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
	// PorkBun API DNS record retrieval URL (domain, subdomain, type)
	retrieveURL = "https://api.porkbun.com/api/json/v3/dns/retrieveByNameType/%s/TXT/%s" // append domain name, subdomain

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

type dnsRetrieveArgs struct {
	APIKey    string `json:"apikey"`
	SecretKey string `json:"secretapikey"`
}

type dnsRetrieveResponse struct {
	Status  string     `json:"status"`
	Records []struct{} `json:"records"`
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

	// check if the record exists before trying to create it
	recordExists, err := c.recordExists(domain, content, subdomain)
	if err != nil {
		return err
	}

	// terminate early if it exists
	if recordExists {
		c.logger.Info("DNS record already exists, skipping creation.")
		return nil
	}

	url := fmt.Sprintf(createURL, domain)

	// create request body
	args := newDNSCreateArgs(c.APIKey, c.SecretKey, subdomain, content)

	body, err := json.Marshal(args)
	if err != nil {
		return err
	}

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

	c.logger.Info("DNS Record created", "domain", domain, "subdomain", subdomain, "content", content)
	return nil
}

// delete an existing DNS record by domain and id
func (c *PorkBunClient) DeleteDNSRecord(domain string, content, subdomain string) error {
	c.logger.Info("Deleting DNS record", "domain", domain, "subdomain", subdomain, "content", content)
	// check if the record exists
	recordExists, err := c.recordExists(domain, content, subdomain)
	if err != nil {
		return err
	}

	// if it doesn't terminate early
	if !recordExists {
		c.logger.Info("Record does not exist, skipping deletion")
		return nil
	}

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

	c.logger.Info("DNS Record deleted successfully", "domain", domain, "subdomain", subdomain, "content", content)
	return nil
}

func (c *PorkBunClient) recordExists(domain string, content string, subdomain string) (bool, error) {
	url := fmt.Sprintf(retrieveURL, domain, subdomain)

	// create request body
	args := dnsRetrieveArgs{
		APIKey:    c.APIKey,
		SecretKey: c.SecretKey,
	}
	body, err := json.Marshal(args)
	if err != nil {
		c.logger.Error(err, "Failed to marshal DNS retrieve request body")
		return false, err
	}

	// create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		c.logger.Error(err, "Failed to create DNS retrieve request.")
		return false, err
	}

	// set headers
	req.Header.Set("Content-Type", "application/json")

	// submit request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error(err, "Failed to submit DNS record retrieve.")
		return false, err
	}

	// check response status code
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		c.logger.Info("Unexpected status code from DNS retrieve response", "status", resp.Status, "response", string(responseBody))
		return false, fmt.Errorf("unexpected status code: %s, response: %s", resp.Status, string(responseBody))
	}

	// parse response
	var response dnsRetrieveResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		c.logger.Error(err, "Failed to decode DNS delete response")
		return false, err
	}

	// check status message, expect success
	if response.Status != "SUCCESS" {
		c.logger.Info("Unexpected DNS retrieve response status", "status", response.Status)
		return false, fmt.Errorf("unexpected response status: %s", response.Status)
	}

	// if the requested DNS record exists, it will be in the records array.
	if len(response.Records) > 0 {
		return true, nil
	}

	return false, nil
}
