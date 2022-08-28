# Unofficial Cert Manager Webhook for Dynu

This is a webhook solver for [Dynu](https://www.dynu.com/).

## Compatibility

* tested with 0.13.0 (might also work for older versions)
* tested with
  - Cert-Manager v1.6.0 operator
  - Kubernetes v1.21.1 / OpenShift 4.8.15

## Installation

```bash
helm repo add cert-manager-dynu-webhook \
 https://dopingus.github.io/cert-manager-webhook-dynu
helm repo update
helm install cert-manager-dynu-webhook cert-manager-dynu-webhook/dynu-webhook
```

## Certificate Issuer

1. Generate an API Key at [Dynu](https://www.dynu.com/en-US/ControlPanel/APICredentials)

2. Create a secret to store your application secret:

    ```bash
    kubectl create secret generic dynu-secret \
      --from-literal=api-key='<DYNU_API_KEY>'
    ```

    The `secretName` can also be changed in `deploy/dynu-webhook/values.yaml` in case you have to follow some convention. 
    The secret must be created in the same namespace as the webhook.

3. Create a certificate issuer:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-dynu-<YOUR_ISSUER_NAME>
spec:
  acme:
    # The ACME server URL
    server: https://acme-v02.api.letsencrypt.org/directory              # Use this for prod
    # server: https://acme-staging-v02.api.letsencrypt.org/directory    # Use this for staging/testing


    # Email address used for ACME registration
    email: example@somedomain # REPLACE THIS WITH YOUR EMAIL!!!

    # Name of a secret used to store the ACME account private key
    privateKeySecretRef:
      name: <YOUR_SECRET_NAME>

    solvers:
      - dns01:
          cnameStrategy: Follow
          webhook:
            groupName: <YOUR_GROUP_NAME> # Use the groupName defined above
            solverName: dynu
            config:
              secretName: dynu-secret # Adjust this in case you changed the secretName
```

## Certificate

Issuing a certificate:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: <CERTIFICATE_NAME>  # Replace with a name of your choice
  namespace: default        # Set a namespace if required
spec:
  commonName: "*.<YOUR_DOMAIN>" # Wildcard Entry for your domain
  dnsNames:
    - <YOUR_DOMAIN>         # List of all (sub)domains that you want to include in the cert
    - "*.<YOUR_DOMAIN>"
  issuerRef:
    name: letsencrypt-dynu-<YOUR_ISSUER_NAME>   # This should match the issuer you defined earlier
    kind: ClusterIssuer
  secretName: <SECRET_NAME> # Secret name where the resulting certificate is saved in
```

## Development

see [webhook-example](https://github.com/cert-manager/webhook-example)

## Test

If you want to run the test
- update testdata/dynu-secret with the correct Dynu API key (base64).

```bash
TEST_ZONE_NAME=your.domain.name. make test
go test -v .
=== RUN   TestRunsSuite
=== RUN   TestRunsSuite/Basic
=== RUN   TestRunsSuite/Basic/PresentRecord
    util.go:68: created fixture "basic-present-record"
    suite.go:37: Calling Present with ChallengeRequest: &v1alpha1.ChallengeRequest{UID:"", Action:"", Type:"", DNSName:"example.com", Key:"123d==", ResourceNamespace:"basic-present-record", ResolvedFQDN:"cert-manager-dns01-tests.your.domain.name.", ResolvedZone:"your.domain.name.", AllowAmbientCredentials:false, Config:(*v1.JSON)(0x40004e3398)}
I0801 22:23:32.050846   29444 main.go:113] call function Present: ResourceNamespace=basic-present-record, ResolvedZone=your.domain.name., ResolvedFQDN=cert-manager-dns01-tests.your.domain.name. DNSName=example.com
I0801 22:23:32.064490   29444 main.go:119] Decoded configuration {dynu-secret}
I0801 22:23:52.811140   29444 main.go:284] Added TXT record result: {"statusCode":200,"id":8718493,"domainId":9754501,"domainName":"your.domain.name","nodeName":"cert-manager-dns01-tests","hostname":"cert-manager-dns01-tests.your.domain.name","recordType":"TXT","ttl":60,"state":true,"content":"cert-manager-dns01-tests.your.domain.name. 60 IN TXT \"123d==\"","updatedOn":"2022-08-02T05:23:52.443","textData":"123d=="}
I0801 22:23:53.820236   29444 main.go:284] Added TXT record result: {"statusCode":200,"id":8718494,"domainId":9754501,"domainName":"your.domain.name","nodeName":"","hostname":"your.domain.name","recordType":"TXT","ttl":60,"state":true,"content":"your.domain.name. 60 IN TXT \"123d==\"","updatedOn":"2022-08-02T05:23:53.573","textData":"123d=="}
I0801 22:23:53.820360   29444 main.go:144] Presented txt record cert-manager-dns01-tests.your.domain.name.
I0801 22:23:58.673091   29444 main.go:196] TXT entry with content your.domain.name. 60 IN TXT "123d==" (key value 123d==)
I0801 22:23:59.301171   29444 main.go:202] Deleted TXT record result: {"statusCode":200}
I0801 22:23:59.302371   29444 main.go:196] TXT entry with content cert-manager-dns01-tests.your.domain.name. 60 IN TXT "123d==" (key value 123d==)
I0801 22:23:59.921555   29444 main.go:202] Deleted TXT record result: {"statusCode":200}
I0801 22:23:59.921671   29444 main.go:196] TXT entry with content your.domain.name. 120 IN SOA ns1.dynu.com. administrator.dynu.com. 0 3600 900 604800 300 (key value 123d==)
I0801 22:24:12.817203   29444 main.go:196] TXT entry with content your.domain.name. 120 IN SOA ns1.dynu.com. administrator.dynu.com. 0 3600 900 604800 300 (key value 123d==)
=== RUN   TestRunsSuite/Extended
=== RUN   TestRunsSuite/Extended/DeletingOneRecordRetainsOthers
    suite.go:73: skipping test as strict mode is disabled, see: https://github.com/cert-manager/cert-manager/pull/1354
--- PASS: TestRunsSuite (165.87s)
    --- PASS: TestRunsSuite/Basic (58.42s)
        --- PASS: TestRunsSuite/Basic/PresentRecord (58.42s)
    --- PASS: TestRunsSuite/Extended (0.00s)
        --- SKIP: TestRunsSuite/Extended/DeletingOneRecordRetainsOthers (0.00s)
PASS
ok      github.com/Dopingus/cert-manager-webhook-dynu   166.121s
```
