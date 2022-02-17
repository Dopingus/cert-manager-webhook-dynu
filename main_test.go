package main

import (
	"os"
	"testing"

	"github.com/jetstack/cert-manager/test/acme/dns"
	"io/ioutil"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"log"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
)

func TestRunsSuite(t *testing.T) {
	// The manifest path should contain a file named config.json that is a
	// snippet of valid configuration that should be included on the
	// ChallengeRequest passed as part of the test cases.
	//
	d, err := ioutil.ReadFile("testdata/dynu/config.json")
	if err != nil {
		log.Fatal(err)
	}

	// Uncomment the below fixture when implementing your custom DNS provider
	fixture := dns.NewFixture(&dynuDNSProviderSolver{},
		dns.SetResolvedZone(zone),
		dns.SetAllowAmbientCredentials(false),
		dns.SetManifestPath("testdata/dynu"),
		dns.SetBinariesPath("_test/kubebuilder/bin"),
		dns.SetUseAuthoritative(true),
		//dns.SetDNSServer("ns4.dynu.com:53"),
		dns.SetManifestPath("testdata/dynu/dynu-secret.yaml"),
		dns.SetConfig(&extapi.JSON{ Raw: d, }),

	)

	fixture.RunConformance(t)
}
