# Unofficial Cert Manager Webhook for Dynu

This is a webhook solver for [Dynu](https://www.dynu.com/).

## Compatibility

* tested with 0.13.0 (might also work for older versions)

## Installation

Define a unique group name for your company or organization (i.e. `acme.mycompany.mydomain`)

```bash
helm install ./deploy/dynu-webhook \
 --set groupName='<YOUR_GROUP_NAME>'
```

Alternatively, modify the `groupName` in `deploy/dynu-webhook/values.yaml`.

## Certificate Issuer

1. Generate an API Key at [Dynu](https://www.dynu.com/en-US/ControlPanel/APICredentials)

2. Create a secret to store your application secret:

    ```bash
    kubectl create secret generic dynu-secret \
      --from-literal=api-key='<DYNU_API_KEY>'
    ```

    The `secretName` can also be changed in `deploy/dynu-webhook/values.yaml` in case you have to follow some convention. 

3. Create a certificate issuer:

    ```yaml
    apiVersion: cert-manager.io/v1alpha2
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
              apiUrl: https://api.dynu.com/v2
    ```

## Certificate

Issuing a certificate:

```yaml
apiVersion: cert-manager.io/v1alpha2
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