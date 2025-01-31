/*
 * Copyright 2020 Netflix, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package creds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/netflix/weep/pkg/httpAuth"
	"github.com/netflix/weep/pkg/httpAuth/custom"

	"github.com/netflix/weep/pkg/util"

	"github.com/netflix/weep/pkg/aws"
	"github.com/netflix/weep/pkg/config"
	werrors "github.com/netflix/weep/pkg/errors"
	"github.com/netflix/weep/pkg/httpAuth/challenge"
	"github.com/netflix/weep/pkg/logging"
	"github.com/netflix/weep/pkg/metadata"

	"github.com/spf13/viper"

	"github.com/pkg/errors"
)

var clientVersion = fmt.Sprintf("%s", metadata.Version)

var userAgent = "weep/" + clientVersion + " Go-http-client/1.1"

// HTTPClient is the interface we expect HTTP clients to implement.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	GetRoleCredentials(role string, ipRestrict bool) (*aws.Credentials, error)
	CloseIdleConnections()
	buildRequest(string, string, io.Reader, string) (*http.Request, error)
}

// Client represents a ConsoleMe client.
type Client struct {
	http.Client
	Host   string
	Region string
}

type Role struct {
	Arn                 string `json:"arn"`
	AccountId           string `json:"account_id"`
	AccountFriendlyName string `json:"account_friendly_name"`
	RoleName            string `json:"role_name"`
}

type response struct {
	Data struct {
		Roles []Role
	}
	Status string
}

// GetClient creates an authenticated ConsoleMe client
func GetClient() (*Client, error) {
	var client *Client
	consoleMeUrl := viper.GetString("consoleme_url")
	httpClient, err := httpAuth.GetAuthenticatedClient()
	if err != nil {
		return client, err
	}
	return NewClient(consoleMeUrl, "", httpClient)
}

// NewClient takes a ConsoleMe hostname and *http.Client, and returns a
// ConsoleMe client that will talk to that ConsoleMe instance for AWS Credentials.
func NewClient(hostname string, region string, httpc *http.Client) (*Client, error) {
	if len(hostname) == 0 {
		return nil, errors.New("hostname cannot be empty string")
	}

	if httpc == nil {
		httpc = &http.Client{Transport: defaultTransport()}
	}

	c := &Client{
		Client: *httpc,
		Host:   hostname,
		Region: region,
	}

	return c, nil
}

func (c *Client) buildRequest(method string, resource string, body io.Reader, apiPrefix string) (*http.Request, error) {
	urlStr := c.Host + apiPrefix + resource
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Add("Content-Type", "application/json")
	err = custom.RunPreflightFunctions(req)
	if err != nil {
		return nil, err
	}

	return req, nil
}

// CloseIdleConnections calls CloseIdleConnections() on the client's HTTP transport.
func (c *Client) CloseIdleConnections() {
	transport, ok := c.Client.Transport.(*http.Transport)
	if !ok {
		// This is unlikely, but we'll fail out anyway.
		return
	}
	transport.CloseIdleConnections()
}

// Roles returns all eligible role ARNs, using v1 of eligible roles endpoint
func (c *Client) Roles() ([]Role, error) {
	req, err := c.buildRequest(http.MethodGet, "/get_roles", nil, "/api/v2")
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}

	// Add URL Parameters
	q := url.Values{}
	q.Add("all", "true")
	req.URL.RawQuery = q.Encode()

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to action request")
	}

	defer resp.Body.Close()
	document, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp.StatusCode, document)
	}

	var jsonData response
	if err := json.Unmarshal(document, &jsonData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON")
	}

	return jsonData.Data.Roles, nil
}

// RolesExtended returns all eligible role along with additional details, using v2 of eligible roles endpoint
func (c *Client) RolesExtended() ([]ConsolemeRolesResponse, error) {
	req, err := c.buildRequest(http.MethodGet, "/get_roles", nil, "/api/v2")
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}

	// Add URL Parameters
	q := url.Values{}
	q.Add("all", "true")
	req.URL.RawQuery = q.Encode()

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to action request")
	}

	defer resp.Body.Close()
	document, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseError(resp.StatusCode, document)
	}

	var responseParsed ConsolemeWebResponse
	if err := json.Unmarshal(document, &responseParsed); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON")
	}
	var roles []ConsolemeRolesResponse
	if err = json.Unmarshal(responseParsed.Data["roles"], &roles); err != nil {
		return nil, werrors.UnexpectedResponseType
	}

	return roles, nil
}

// GetResourceURL gets resource URL from ConsoleMe given an ARN
func (c *Client) GetResourceURL(arn string) (string, error) {
	req, err := c.buildRequest(http.MethodGet, "/get_resource_url", nil, "/api/v2")
	if err != nil {
		return "", errors.Wrap(err, "failed to build request")
	}

	// Add URL Parameters
	q := url.Values{}
	q.Add("arn", arn)
	req.URL.RawQuery = q.Encode()

	resp, err := c.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "failed to action request")
	}

	defer resp.Body.Close()
	document, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "failed to read response body")
	}
	if resp.StatusCode != http.StatusOK {
		return "", parseWebError(document)
	}
	var responseParsed ConsolemeWebResponse
	if err := json.Unmarshal(document, &responseParsed); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal JSON")
	}
	var respURL string
	if err = json.Unmarshal(responseParsed.Data["url"], &respURL); err != nil {
		return "", werrors.UnexpectedResponseType
	}
	return config.BaseWebURL() + respURL, nil
}

// GenericGet makes a GET request to the request URL
func (c *Client) GenericGet(resource string, apiPrefix string) (map[string]json.RawMessage, error) {
	return c.genericRequest(http.MethodGet, resource, apiPrefix, nil)
}

// GenericPost makes a POST request to the request URL
func (c *Client) GenericPost(resource string, apiPrefix string, b *bytes.Buffer) (map[string]json.RawMessage, error) {
	return c.genericRequest(http.MethodPost, resource, apiPrefix, b)
}

func (c *Client) genericRequest(method string, resource string, apiPrefix string, b io.Reader) (map[string]json.RawMessage, error) {
	req, err := c.buildRequest(method, resource, b, apiPrefix)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to action request")
	}

	defer resp.Body.Close()
	document, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseWebError(document)
	}
	var responseParsed ConsolemeWebResponse
	if err := json.Unmarshal(document, &responseParsed); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON")
	}

	return responseParsed.Data, nil
}

func parseWebError(rawErrorResponse []byte) error {
	var errorResponse ConsolemeWebResponse
	if err := json.Unmarshal(rawErrorResponse, &errorResponse); err != nil {
		return errors.Wrap(err, "failed to unmarshal JSON")
	}
	return fmt.Errorf(strings.Join(errorResponse.Errors, "\n"))
}

func parseError(statusCode int, rawErrorResponse []byte) error {
	var errorResponse ConsolemeCredentialErrorMessageType
	if err := json.Unmarshal(rawErrorResponse, &errorResponse); err != nil {
		return errors.Wrap(err, "failed to unmarshal JSON")
	}

	switch errorResponse.Code {
	case "899":
		return werrors.InvalidArn
	case "900":
		return werrors.NoMatchingRoles
	case "901":
		return werrors.MultipleMatchingRoles
	case "902":
		return werrors.CredentialRetrievalError
	case "903":
		return werrors.NoMatchingRoles
	case "904":
		return werrors.MalformedRequestError
	case "905":
		return werrors.MutualTLSCertNeedsRefreshError
	case "invalid_jwt":
		logging.Log.Errorf("Authentication is invalid or has expired. Please restart weep to re-authenticate.")
		err := challenge.DeleteLocalWeepCredentials()
		if err != nil {
			logging.Log.Errorf("failed to delete credentials: %v", err)
		}
		return werrors.InvalidJWT
	default:
		return fmt.Errorf("unexpected HTTP status %d, want 200. Response: %s", statusCode, string(rawErrorResponse))
	}
}

func (c *Client) GetRoleCredentials(role string, ipRestrict bool) (*aws.Credentials, error) {
	return getRoleCredentialsFunc(c, role, ipRestrict)
}

func (c *Client) GetAccounts(query string) ([]ConsolemeAccountDetails, error) {
	resp, err := c.searchResources("account", query, 1000)
	if err != nil {
		return nil, err
	}
	var accounts []ConsolemeAccountDetails
	for _, account := range resp {
		idx := strings.Index(account.Title, "(")
		accountName := account.Title[0 : idx-1]
		accountNum := account.Title[idx+1 : strings.Index(account.Title, ")")]
		accounts = append(accounts, ConsolemeAccountDetails{AccountName: accountName, AccountNumber: accountNum})
	}
	return accounts, nil
}

func (c *Client) GetRolesInAccount(query string, accountNumber string) ([]ConsolemeRolesResponse, error) {
	query = "arn:aws:iam::" + accountNumber + ":role/" + query
	resp, err := c.searchResources("iam_arn", query, 5000)
	if err != nil {
		return nil, err
	}
	var roles []ConsolemeRolesResponse
	for _, role := range resp {
		arn, _ := util.ArnParse(role.Title)
		roles = append(roles, ConsolemeRolesResponse{Arn: role.Title, RoleName: arn.Resource})
	}
	return roles, nil
}

func (c *Client) searchResources(resourceType string, query string, limit int) ([]ConsolemeResourceSearchResponseElement, error) {
	req, err := c.buildRequest(http.MethodGet, "/policies/typeahead", nil, "/api/v1")
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}

	// Add URL Parameters
	q := url.Values{}
	q.Add("search", query)
	q.Add("resource", resourceType)
	q.Add("limit", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()

	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to action request")
	}

	defer resp.Body.Close()
	document, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseWebError(document)
	}

	var responseParsed []ConsolemeResourceSearchResponseElement
	if err := json.Unmarshal(document, &responseParsed); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON")
	}

	return responseParsed, nil
}

func getRoleCredentialsFunc(c HTTPClient, role string, ipRestrict bool) (*aws.Credentials, error) {
	var credentialsResponse ConsolemeCredentialResponseType

	cmCredRequest := ConsolemeCredentialRequestType{
		RequestedRole:  role,
		NoIpRestricton: ipRestrict,
	}

	if metadataEnabled := viper.GetBool("feature_flags.consoleme_metadata"); metadataEnabled == true {
		cmCredRequest.Metadata = metadata.GetInstanceInfo()
	}

	b := new(bytes.Buffer)
	err := json.NewEncoder(b).Encode(cmCredRequest)
	if err != nil {
		return credentialsResponse.Credentials, errors.Wrap(err, "failed to create request body")
	}

	req, err := c.buildRequest(http.MethodPost, "/get_credentials", b, "/api/v1")
	if err != nil {
		return credentialsResponse.Credentials, errors.Wrap(err, "failed to build request")
	}

	resp, err := c.Do(req)
	if err != nil {
		return credentialsResponse.Credentials, errors.Wrap(err, "failed to action request")
	}

	defer resp.Body.Close()
	document, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return credentialsResponse.Credentials, errors.Wrap(err, "failed to read response body")
	}
	if resp.StatusCode != http.StatusOK {
		return credentialsResponse.Credentials, parseError(resp.StatusCode, document)
	}

	if err := json.Unmarshal(document, &credentialsResponse); err != nil {
		return credentialsResponse.Credentials, errors.Wrap(err, "failed to unmarshal JSON")
	}

	if credentialsResponse.Credentials == nil {
		return nil, werrors.CredentialRetrievalError
	}

	return credentialsResponse.Credentials, nil
}

func defaultTransport() *http.Transport {
	timeout := time.Duration(viper.GetInt("server.http_timeout")) * time.Second
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
	}
}

type ClientMock struct {
	DoFunc                 func(req *http.Request) (*http.Response, error)
	GetRoleCredentialsFunc func(role string, ipRestrict bool) (*aws.Credentials, error)
}

func (c *ClientMock) GetRoleCredentials(role string, ipRestrict bool) (*aws.Credentials, error) {
	return getRoleCredentialsFunc(c, role, ipRestrict)
}

func (c *ClientMock) CloseIdleConnections() {}

func (c *ClientMock) buildRequest(string, string, io.Reader, string) (*http.Request, error) {
	return &http.Request{}, nil
}

func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	return c.DoFunc(req)
}

func GetTestClient(responseBody interface{}) (HTTPClient, error) {
	var responseCredentials *aws.Credentials
	var responseCode = 200
	if c, ok := responseBody.(ConsolemeCredentialResponseType); ok {
		responseCredentials = c.Credentials
	}
	if e, ok := responseBody.(ConsolemeCredentialErrorMessageType); ok {
		code, err := strconv.Atoi(e.Code)
		if err == nil {
			responseCode = code
		}
	}
	resp, err := json.Marshal(responseBody)
	if err != nil {
		return nil, err
	}
	var client HTTPClient
	client = &ClientMock{
		DoFunc: func(*http.Request) (*http.Response, error) {
			r := ioutil.NopCloser(bytes.NewReader(resp))
			return &http.Response{
				StatusCode: responseCode,
				Body:       r,
			}, nil
		},
		GetRoleCredentialsFunc: func(role string, ipRestrict bool) (*aws.Credentials, error) {
			if responseCredentials != nil {
				return responseCredentials, nil
			}
			return &aws.Credentials{RoleArn: role}, nil
		},
	}
	return client, nil
}

// GetCredentialsC uses the provided Client to request credentials from ConsoleMe then
// follows the provided chain of roles to assume. Roles are assumed in the order in which
// they appear in the assumeRole slice.
func GetCredentialsC(client HTTPClient, role string, ipRestrict bool, assumeRole []string) (*aws.Credentials, error) {
	resp, err := client.GetRoleCredentials(role, ipRestrict)
	if err != nil {
		return nil, err
	}

	for _, assumeRoleArn := range assumeRole {
		resp.AccessKeyId, resp.SecretAccessKey, resp.SessionToken, err = aws.GetAssumeRoleCredentials(resp.AccessKeyId, resp.SecretAccessKey, resp.SessionToken, assumeRoleArn)
		if err != nil {
			return nil, fmt.Errorf("role assumption failed for %s: %s", assumeRoleArn, err)
		}
	}

	return resp, nil
}

// GetCredentials requests credentials from ConsoleMe then follows the provided chain of roles to
// assume. Roles are assumed in the order in which they appear in the assumeRole slice.
func GetCredentials(role string, ipRestrict bool, assumeRole []string, region string) (*aws.Credentials, error) {
	client, err := GetClient()
	if err != nil {
		return nil, err
	}

	return GetCredentialsC(client, role, ipRestrict, assumeRole)
}
