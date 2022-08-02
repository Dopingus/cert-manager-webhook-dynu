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
 https://anon-software.github.io/cert-manager-webhook-dynu
helm repo update
helm install cert-manager-dynu-webhook/dynu-webhook
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
              zoneName: <YOUR_DOMAIN> # Add the domain which you want to create certiciates for
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
- update testdata/config.json with the your domain name (zoneName)

```bash
TEST_ZONE_NAME=your.domain.name. make test
go test -v .
=== RUN   TestRunsSuite
    fixture.go:117: started apiserver on "http://127.0.0.1:36591"
=== RUN   TestRunsSuite/Conformance
=== RUN   TestRunsSuite/Conformance/Basic
=== RUN   TestRunsSuite/Conformance/Basic/PresentRecord
I0217 17:23:26.141761  750764 request.go:1097] Request Body: {"kind":"Namespace","apiVersion":"v1","metadata":{"name":"basic-present-record","creationTimestamp":null},"spec":{},"status":{}}
I0217 17:23:26.141879  750764 round_trippers.go:423] curl -k -v -XPOST  -H "User-Agent: cert-manager-webhook-dynu.test/v0.0.0 (linux/amd64) kubernetes/$Format" -H "Accept: application/json, */*" -H "Content-Type: application/json" 'http://127.0.0.1:36591/api/v1/namespaces'
I0217 17:23:26.143894  750764 round_trippers.go:443] POST http://127.0.0.1:36591/api/v1/namespaces 201 Created in 1 milliseconds
I0217 17:23:26.143918  750764 round_trippers.go:449] Response Headers:
I0217 17:23:26.143925  750764 round_trippers.go:452]     Content-Type: application/json
I0217 17:23:26.143930  750764 round_trippers.go:452]     Date: Thu, 17 Feb 2022 16:23:26 GMT
I0217 17:23:26.143935  750764 round_trippers.go:452]     Content-Length: 311
I0217 17:23:26.143940  750764 round_trippers.go:452]     Cache-Control: no-cache, private
I0217 17:23:26.143993  750764 request.go:1097] Response Body: {"kind":"Namespace","apiVersion":"v1","metadata":{"name":"basic-present-record","selfLink":"/api/v1/namespaces/basic-present-record","uid":"41f5aa15-2d84-43bd-ab3b-779de179fd05","resourceVersion":"45","creationTimestamp":"2022-02-17T16:23:26Z"},"spec":{"finalizers":["kubernetes"]},"status":{"phase":"Active"}}
...
I0217 17:23:44.233823  750764 request.go:1097] Request Body: {"kind":"DeleteOptions","apiVersion":"v1"}
I0217 17:23:44.233884  750764 round_trippers.go:423] curl -k -v -XDELETE  -H "User-Agent: cert-manager-webhook-dynu.test/v0.0.0 (linux/amd64) kubernetes/$Format" -H "Accept: application/json, */*" -H "Content-Type: application/json" 'http://127.0.0.1:36591/api/v1/namespaces/basic-present-record'
I0217 17:23:44.235897  750764 round_trippers.go:443] DELETE http://127.0.0.1:36591/api/v1/namespaces/basic-present-record 200 OK in 1 milliseconds
I0217 17:23:44.235912  750764 round_trippers.go:449] Response Headers:
I0217 17:23:44.235920  750764 round_trippers.go:452]     Cache-Control: no-cache, private
I0217 17:23:44.235925  750764 round_trippers.go:452]     Content-Type: application/json
I0217 17:23:44.235932  750764 round_trippers.go:452]     Date: Thu, 17 Feb 2022 16:23:44 GMT
I0217 17:23:44.235938  750764 round_trippers.go:452]     Content-Length: 359
I0217 17:23:44.235967  750764 request.go:1097] Response Body: {"kind":"Namespace","apiVersion":"v1","metadata":{"name":"basic-present-record","selfLink":"/api/v1/namespaces/basic-present-record","uid":"41f5aa15-2d84-43bd-ab3b-779de179fd05","resourceVersion":"48","creationTimestamp":"2022-02-17T16:23:26Z","deletionTimestamp":"2022-02-17T16:23:44Z"},"spec":{"finalizers":["kubernetes"]},"status":{"phase":"Terminating"}}
=== RUN   TestRunsSuite/Conformance/Extended
=== RUN   TestRunsSuite/Conformance/Extended/DeletingOneRecordRetainsOthers
    suite.go:73: skipping test as strict mode is disabled, see: https://github.com/jetstack/cert-manager/pull/1354
--- PASS: TestRunsSuite (24.78s)
    --- PASS: TestRunsSuite/Conformance (18.09s)
        --- PASS: TestRunsSuite/Conformance/Basic (18.09s)
            --- PASS: TestRunsSuite/Conformance/Basic/PresentRecord (18.09s)
        --- PASS: TestRunsSuite/Conformance/Extended (0.00s)
            --- SKIP: TestRunsSuite/Conformance/Extended/DeletingOneRecordRetainsOthers (0.00s)
PASS
ok  	github.com/cert-manager/cert-manager-webhook-dynu	24.867s
```
