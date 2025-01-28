package porkbunclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// PorkBun API DNS record creation URL
	createURL = "https://porkbun.com/api/json/v3/dns/create/%s" // append domain name

	// PorkBun API DNS record deletion URL
	deleteURL = "https://porkbun.com/api/json/v3/dns/delete/%s/%s" // append domain name, record ID
)

type PorkBunClient struct {
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
	TTL       int    `json:"ttl"`
}

type dnsCreateResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

type dnsDeleteArgs struct {
	APIKey    string `json:"apikey"`
	SecretKey string `json:"secretapikey"`
}

type dnsDeleteResponse struct {
	Status string `json:"status"`
}

func newDNSCreateArgs(apiKey string, secretKey string, name string, content string) *dnsCreateArgs {
	return &dnsCreateArgs{
		APIKey:    apiKey,
		SecretKey: secretKey,
		Name:      name,
		Type:      "TXT",
		Content:   content,
		TTL:       600,
	}
}

// constructor for porkbun client
func NewPorkBunClient(apiKey string, secretKey string) *PorkBunClient {
	return &PorkBunClient{
		httpClient: http.Client{Timeout: 10 * time.Second},
		APIKey:     apiKey,
		SecretKey:  secretKey,
	}
}

// create a new DNS record, returning the ID of the created record
func (c *PorkBunClient) CreateDNSRecord(domain string, content string, subdomain string) (string, error) {
	url := fmt.Sprintf(createURL, domain)

	// create request body
	args := newDNSCreateArgs(c.APIKey, c.SecretKey, subdomain, content)
	body, err := json.Marshal(args)
	if err != nil {
		return "", err
	}

	// create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	// set request headers
	req.Header.Set("Content-Type", "application/json")

	// send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// check response status code
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %s, response: %s", resp.Status, string(responseBody))
	}

	// parse response
	var response dnsCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}

	if response.Status != "SUCCESS" {
		return "", fmt.Errorf("unexpected response status: %s", response.Status)
	}

	return response.ID, nil
}

// delete an existing DNS record by domain and id
func (c *PorkBunClient) DeleteDNSRecord(domain string, id string) error {
	url := fmt.Sprintf(deleteURL, domain, id)

	deleteArgs := dnsDeleteArgs{
		APIKey:    c.APIKey,
		SecretKey: c.SecretKey,
	}

	// create request body
	body, err := json.Marshal(deleteArgs)
	if err != nil {
		return nil
	}

	// create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))

	// set request headers
	req.Header.Set("Content-Type", "application/json")

	// send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	// check response status code
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %s, response: %s", resp.Status, string(responseBody))
	}

	// parse response
	var response dnsDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return err
	}

	if response.Status != "SUCCESS" {
		return fmt.Errorf("unexpected response status: %s", response.Status)
	}

	return nil
}
