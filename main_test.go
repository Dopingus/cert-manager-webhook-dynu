package main

import (
	"os"
	"testing"

	//"github.com/cert-manager/cert-manager/test/acme/dns"
	acmetest "github.com/cert-manager/cert-manager/test/acme"
)

var (
	zone = os.Getenv("TEST_ZONE_NAME")
)

func TestRunsSuite(t *testing.T) {
	// The manifest path should contain a file named config.json that is a
	// snippet of valid configuration that should be included on the
	// ChallengeRequest passed as part of the test cases.
	//
	//d, err := ioutil.ReadFile("testdata/dynu/config.json")
	//if err != nil {
	//	log.Fatal(err)
	//}

	//	os.Setenv("TEST_ASSET_ETCD", "_test/kubebuilder/bin/etcd")
	//	os.Setenv("TEST_ASSET_KUBE_APISERVER", "_test/kubebuilder/bin/kube-apiserver")
	//	defer os.Unsetenv("TEST_ASSET_ETCD")
	//	defer os.Unsetenv("TEST_ASSET_KUBE_APISERVER")

	// Uncomment the below fixture when implementing your custom DNS provider
	//fixture := acmetest.NewFixture(&customDNSProviderSolver{},
	//	acmetest.SetResolvedZone(zone),
	//	acmetest.SetAllowAmbientCredentials(false),
	//	acmetest.SetManifestPath("testdata/my-custom-solver"),
	//	acmetest.SetBinariesPath("_test/kubebuilder/bin"),
	//)
	fixture := acmetest.NewFixture(&dynuDNSProviderSolver{},
		acmetest.SetResolvedZone(zone),
		acmetest.SetAllowAmbientCredentials(false),
		acmetest.SetUseAuthoritative(true),
		//acmetest.SetDNSServer("ns4.dynu.com:53"),
		//acmetest.SetManifestPath("testdata/dynu/dynu-secret.yaml"),
		acmetest.SetManifestPath("testdata/dynu"),
		//acmetest.SetConfig(&extapi.JSON{Raw: d}),
	)

	//need to uncomment and  RunConformance delete runBasic and runExtended once https://github.com/cert-manager/cert-manager/pull/4835 is merged
	//fixture.RunConformance(t)
	fixture.RunBasic(t)
	//fixture.RunExtended(t)

}
