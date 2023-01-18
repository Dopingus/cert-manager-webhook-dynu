package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"github.com/cert-manager/cert-manager/pkg/issuer/acme/dns/util"
)

const (
	apiUrl = "https://api.dynu.com/v2"
)

type DnsRecordResponse struct {
	DnsRecords []DnsRecord `json:"dnsRecords"`
}

type DnsRecord struct {
	Id         int    `json:"id"`
	DomainId   int    `json:"domainId"`
	NodeName   string `json:"nodeName"`
	RecordType string `json:"recordType"`
	Ttl        int    `json:"ttl"`
	Content    string `json:"content"`
	UpdatedOn  string `json:"updatedOn"`
	TextData   string `json:"textData"`
}

type DNSRootResponse struct {
	Id         int    `json:"id"`
	DomainName string `json:"domainName"`
	Hostname   string `json:"hostname"`
	Node       string `json:"node"`
}

type DNSSubResponse struct {
	Id         int    `json:"id"`
	DomainName string `json:"domainName"`
	Node       string `json:"node"`
}

type DomainRecordResponse struct {
	Domains []Domain `json:"domains"`
}

type Domain struct {
	Id                int    `json:"id"`
	Name              string `json:"name"`
	UnicodeName       string `json:"unicodeName"`
	Token             string `json:"token"`
	State             string `json:"state"`
	Group             string `json:"group"`
	Ipv4Address       string `json:"ipv4Address"`
	Ipv6Address       string `json:"ipv6Address"`
	Ttl               int    `json:"ttl"`
	Ipv4              bool   `json:"ipv4"`
	Ipv6              bool   `json:"ipv6"`
	Ipv4WildcardAlias bool   `json:"ipv4WildcardAlias"`
	Ipv6WildcardAlias bool   `json:"ipv6WildcardAlias"`
	CreatedOn         string `json:"createdOn"`
	UpdatedOn         string `json:"updatedOn"`
}

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&dynuDNSProviderSolver{},
	)
}

// customDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/jetstack/cert-manager/pkg/acme/webhook.Solver`
// interface.
type dynuDNSProviderSolver struct {
	client *kubernetes.Clientset
}

// customDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type dynuDNSProviderConfig struct {
	// These fields will be set by users in the
	// `issuer.spec.acme.dns01.providers.webhook.config` field.
	SecretRef string `json:"secretName"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
func (c *dynuDNSProviderSolver) Name() string {
	return "dynu"
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *dynuDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.Infof("call function Present: ResourceNamespace=%s, ResolvedZone=%s, ResolvedFQDN=%s DNSName=%s", ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN, ch.DNSName)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	klog.Infof("Decoded configuration %v", cfg)

	secretName := cfg.SecretRef
	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get secret `%s/%s ; %v`", secretName, ch.ResourceNamespace, err)
	}

	apiKey, err := stringFromSecretData(&sec.Data, "api-key")
	if err != nil {
		return fmt.Errorf("unable to get api-key from secret %v", err)
	}

	domainId, recordName, err := getDomainIdFromFQDN(apiKey, ch.ResolvedFQDN)
	if err != nil {
		return err
	}

	baseRecordName := determineBaseRecordName(recordName)

	// For requested record
	addTxtRecord(apiKey, domainId, recordName, ch)
	// For record name without _acme-challenge as well (DNS propagation is checked through this name)
	addTxtRecord(apiKey, domainId, baseRecordName, ch)

	klog.Infof("Presented txt record %v", ch.ResolvedFQDN)

	return nil
}

func determineBaseRecordName(recordName string) string {
	klog.Infof("call function determineBaseRecordName: recordName=%s", recordName)
	splitRecordName := strings.SplitN(recordName, ".", 2)
	if len(splitRecordName) > 1 {
		return splitRecordName[len(splitRecordName)-1]
	} else {
		return ""
	}
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *dynuDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	secretName := cfg.SecretRef

	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get secret `%s/%s ; %v`", secretName, ch.ResourceNamespace, err)
	}
	apiKey, err := stringFromSecretData(&sec.Data, "api-key")
	if err != nil {
		return fmt.Errorf("unable to get api-key from secret `%s/%s` ; %v", secretName, ch.ResourceNamespace, err)
	}

	domainId, _, err := getDomainIdFromFQDN(apiKey, ch.ResolvedFQDN)
	if err != nil {
		return fmt.Errorf("unable to retrieve domainId for domain name %s ; %v", ch.DNSName, err)
	}

	dnsRecords, err := getRecordsForDomain(apiKey, domainId)
	if err != nil {
		return fmt.Errorf("unable to get DNS records %v", err)
	}
	dnsRecordsResponse := DnsRecordResponse{}
	readErr := json.Unmarshal(dnsRecords, &dnsRecordsResponse)

	if readErr != nil {
		return fmt.Errorf("unable to unmarshal response %v", readErr)
	}

	for i := len(dnsRecordsResponse.DnsRecords) - 1; i >= 0; i-- {
		klog.Infof("TXT entry with content %s (key value %s)", dnsRecordsResponse.DnsRecords[i].Content, ch.Key)
		if dnsRecordsResponse.DnsRecords[i].RecordType == "TXT" && dnsRecordsResponse.DnsRecords[i].TextData == ch.Key {
			deleteResponse, err := deleteTxtRecord(apiKey, domainId, dnsRecordsResponse.DnsRecords[i].Id)
			if err != nil {
				klog.Error(err)
			}
			klog.Infof("Deleted TXT record result: %s", deleteResponse)
		}
	}

	return nil
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *dynuDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	klog.V(6).Infof("Input variable stopCh is %d length", len(stopCh))
	if err != nil {
		return err
	}

	c.client = cl

	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (dynuDNSProviderConfig, error) {
	cfg := dynuDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}

func getDomainIdFromFQDN(apiKey string, ResolvedFQDN string) (string, string, error) {
	klog.Infof("call function getDomainIdFromFQDN: apiKey=%s, ResolvedFQDN=%s", apiKey, ResolvedFQDN)
	hostname := util.UnFqdn(ResolvedFQDN)
	url := apiUrl + "/dns/getroot/" + hostname
	response, err := callDnsApi(url, "GET", nil, apiKey)
	if err != nil {
		return "", "", err
	}

	dnsRootResponse := DNSRootResponse{}
	readErr := json.Unmarshal(response, &dnsRootResponse)
	if readErr != nil {
		return "", "", readErr
	}

	// set root response domain values as default
	domainId := dnsRootResponse.Id
	domainNode := dnsRootResponse.Node

	// adding logic here of a simple test to determine if the node has a portion of domain identifier by checking for a period
	if strings.Contains(dnsRootResponse.Node, ".") {
		klog.Infof("Return node name shows that a subdomain could have been specified: Node=%s", dnsRootResponse.Node)

		subFound, subResponse := getSubDomainId(apiKey, ResolvedFQDN)
		if subFound {
			domainId = subResponse.Id
			domainNode = subResponse.Node
		}
	}

	klog.Infof("Domain Detail: id=%s, node=%s", strconv.Itoa(domainId), domainNode)
	return fmt.Sprint(domainId), domainNode, nil
}

// Function looks for the top level sub domain name
func getSubDomainId(apiKey string, fqdn string) (bool, DNSSubResponse) {
	matchName := false
	subResponse := DNSSubResponse{}

	// get a list of the domains for the API key to check for subdomain match
	domainRecords, err := getDomains(apiKey)
	if err != nil {
		klog.Infof("unable to get Domain records %v", err)
		return matchName, subResponse
	}
	domainRecordsResponse := DomainRecordResponse{}
	readErr := json.Unmarshal(domainRecords, &domainRecordsResponse)

	if readErr != nil {
		klog.Infof("unable to unmarshal response %v", readErr)
		return matchName, subResponse
	}

	parts := strings.Split(fqdn, ".")
	for i := range parts {
		domain := strings.Join(parts[i:], ".")
		domain = strings.TrimSuffix(domain, ".")
		for k, record := range domainRecordsResponse.Domains {
			klog.Infof("Checking domain %s with subdmain %s", record.Name, domain)
			if record.Name == domain {
				klog.Infof("Subdomain match found %v", k)
				fqdn = strings.TrimSuffix(fqdn, ".")
				subResponse.Id = record.Id
				subResponse.Node = strings.TrimSuffix(fqdn, "." + record.Name)
				return true, subResponse
			}
		}
	}

	if !matchName {
		klog.Infof("Sub domain match not found")
	}

	return matchName, subResponse
}

func stringFromSecretData(secretData *map[string][]byte, key string) (string, error) {
	data, ok := (*secretData)[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret data", key)
	}
	return string(data), nil
}

func addTxtRecord(apiKey string, domainId string, recordName string, ch *v1alpha1.ChallengeRequest) {
	requestbody := map[string]string{
		"nodeName":   recordName,
		"recordType": "TXT",
		"ttl":        "60",
		"group":      "",
		"state":      "true",
		"textData":   ch.Key}
	jsonBody, _ := json.Marshal(requestbody)
	url := apiUrl + "/dns/" + domainId + "/record"
	response, err := callDnsApi(url, "POST", bytes.NewBuffer(jsonBody), apiKey)

	if err != nil {
		klog.Error(err)
	}
	klog.Infof("Added TXT record result: %s", string(response))
}

// Get a list of the Domains associated with the API to allow for an enumerated check (DYNU API does not have any subdomain filtering)
func getDomains(apiKey string) ([]byte, error) {
	url := apiUrl + "/dns"
	response, err := callDnsApi(url, "GET", nil, apiKey)

	return response, err
}

func getRecordsForDomain(apiKey string, domainId string) ([]byte, error) {
	url := apiUrl + "/dns/" + domainId + "/record"
	response, err := callDnsApi(url, "GET", nil, apiKey)

	return response, err
}

func deleteTxtRecord(apiKey string, domainId string, recordId int) (string, error) {
	url := apiUrl + "/dns/" + domainId + "/record/" + fmt.Sprint(recordId)
	response, err := callDnsApi(url, "DELETE", nil, apiKey)

	return string(response), err
}

func callDnsApi(url string, method string, body io.Reader, apiKey string) ([]byte, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return []byte{}, fmt.Errorf("unable to execute request %v", err)
	}
	req.Close = true
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("API-Key", apiKey)
	t := &http.Transport{
		TLSHandshakeTimeout: 60 * time.Second,
	}
	client := &http.Client{
		Transport: t,
	}
	resp, err := client.Do(req)
	if err != nil {
		klog.Errorf("Failed to Do request")
		return nil, err
	}

	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return respBody, nil
	}

	text := "Error calling API status:" + resp.Status + " url: " + url + " method: " + method
	klog.Error(text)
	return nil, errors.New(text)
}
