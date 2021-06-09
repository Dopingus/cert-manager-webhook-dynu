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
	"regexp"
	"strings"
	"time"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
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

type DomainResponse struct {
	Domains []DomainRecord `json:"domains"`
}

type DomainRecord struct {
	Id        int    `json:"id"`
	Name      string `json:"name"`
	Ttl       int    `json:"ttl"`
	UpdatedOn string `json:"updatedOn"`
}

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&dynuDNSProviderSolver{},
	)
}

type dynuDNSProviderSolver struct {
	client *kubernetes.Clientset
}

type dynuDNSProviderConfig struct {
	SecretRef string `json:"secretName"`
	ZoneName  string `json:"zoneName"`
	ApiUrl    string `json:"apiUrl"`
}

func (c *dynuDNSProviderSolver) Name() string {
	return "dynu"
}

func (c *dynuDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.Infof("call function Present: namespace=%s, zone=%s, fqdn=%s", ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	klog.Infof("Decoded configuration %v", cfg)

	secretName := cfg.SecretRef
	apiUrl := cfg.ApiUrl
	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get secret `%s/%s ; %v`", secretName, ch.ResourceNamespace, err)
	}

	apiKey, err := stringFromSecretData(&sec.Data, "api-key")
	if err != nil {
		return fmt.Errorf("unable to get api-key from secret %v", err)
	}
	zoneName := cfg.ZoneName
	zoneId, err := getZoneIdFromZoneName(apiUrl, apiKey, zoneName)
	if err != nil {
		return err
	}

	recordName := determineRecordName(zoneName, ch.ResolvedFQDN)
	baseRecordName := determineBaseRecordName(recordName)

	// For requested record
	addTxtRecord(apiUrl, apiKey, zoneId, recordName, ch)
	// For record name without _acme-challenge as well (DNS propagation is checked through this name)
	addTxtRecord(apiUrl, apiKey, zoneId, baseRecordName, ch)

	klog.Infof("Presented txt record %v", ch.ResolvedFQDN)

	return nil
}

func determineBaseRecordName(recordName string) string {
	splitRecordName := strings.SplitN(recordName, ".", 2)
	if len(splitRecordName) > 1 {
		return splitRecordName[len(splitRecordName)-1]
	} else {
		return ""
	}
}

func (c *dynuDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	apiUrl := cfg.ApiUrl
	secretName := cfg.SecretRef
	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get secret `%s/%s ; %v`", secretName, ch.ResourceNamespace, err)
	}
	apiKey, err := stringFromSecretData(&sec.Data, "api-key")
	if err != nil {
		return fmt.Errorf("unable to get api-key from secret `%s/%s` ; %v", secretName, ch.ResourceNamespace, err)
	}

	zoneName := cfg.ZoneName
	zoneId, err := getZoneIdFromZoneName(apiUrl, apiKey, zoneName)
	if err != nil {
		return fmt.Errorf("unable to retrieve zoneId for zoneName %s ; %v", zoneName, err)
	}

	dnsRecords, err := getRecordsForDomain(apiUrl, apiKey, zoneId)
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
			deleteResponse, err := deleteTxtRecord(apiUrl, apiKey, zoneId, dnsRecordsResponse.DnsRecords[i].Id)
			if err != nil {
				klog.Error(err)
			}
			klog.Infof("Deleted TXT record result: %s", deleteResponse)
		}
	}

	return nil
}

func (c *dynuDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	klog.V(6).Infof("Input variable stopCh is %d length", len(stopCh))
	if err != nil {
		return err
	}

	c.client = cl

	return nil
}

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

func determineRecordName(zoneName string, fqdn string) string {
	r := regexp.MustCompile("(.+)\\." + zoneName + "\\.")
	name := r.FindStringSubmatch(fqdn)
	if len(name) != 2 {
		klog.Errorf("splitting domain name %s failed!", fqdn)
		return ""
	}
	return name[1]
}

func getZoneIdFromZoneName(apiUrl string, apiKey string, zoneName string) (string, error) {
	url := apiUrl + "/dns"
	response, err := callDnsApi(url, "GET", nil, apiKey)
	if err != nil {
		return "", err
	}

	domainResponse := DomainResponse{}
	readErr := json.Unmarshal(response, &domainResponse)
	if readErr != nil {
		return "", readErr
	}

	for _, domain := range domainResponse.Domains {
		if domain.Name == zoneName {
			return fmt.Sprint(domain.Id), nil
		}
	}

	return "", fmt.Errorf(`zone "%s" could not be found in managed domains`, zoneName)
}

func stringFromSecretData(secretData *map[string][]byte, key string) (string, error) {
	data, ok := (*secretData)[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret data", key)
	}
	return string(data), nil
}

func addTxtRecord(apiUrl string, apiKey string, zoneId string, recordName string, ch *v1alpha1.ChallengeRequest) {
	requestbody := map[string]string{
		"nodeName":   recordName,
		"recordType": "TXT",
		"ttl":        "60",
		"group":      "",
		"state":      "true",
		"textData":   ch.Key}
	jsonBody, _ := json.Marshal(requestbody)
	url := apiUrl + "/dns/" + zoneId + "/record"
	response, err := callDnsApi(url, "POST", bytes.NewBuffer(jsonBody), apiKey)

	if err != nil {
		klog.Error(err)
	}
	klog.Infof("Added TXT record result: %s", string(response))
}

func getRecordsForDomain(apiUrl string, apiKey string, zoneId string) ([]byte, error) {
	url := apiUrl + "/dns/" + zoneId + "/record"
	response, err := callDnsApi(url, "GET", nil, apiKey)

	return response, err
}

func deleteTxtRecord(apiUrl string, apiKey string, zoneId string, recordId int) (string, error) {
	url := apiUrl + "/dns/" + zoneId + "/record/" + fmt.Sprint(recordId)
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
