# K8s Mutating Webhook that adds priorityClassName to Deployments

## Overview

This project implements a Kubernetes Admission Control Webhook that leverages the [MutatingAdmissionWebhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/) Controller.
The webhook intercepts Deployment CREATE and UPDATE K8s API requests and adds a priorityClassName (eg: ```priorityClassName=high-priority-nonpreempting```) and annotation (eg: ```priorityClassWebhook/updated_at: Tue Aug 29 23:55:09 AEST 2023```).

## Admission Controllers and webhooks in the K8s Architecture

![Admission Controllers and webhooks in K8s Architecture](./admission_controller.jpeg "Admission Controllers and webhooks in K8s Architecture")

## Prerequisites

Before getting started with the webhook, ensure that the following tools and resources are available:

- **Docker**: The webhook runs as a container, so Docker is necessary.
- **Kubernetes Cluster**: You'll need a running Kubernetes cluster where the webhook will be deployed.
   - Use my [terraform code](https://github.com/andreistefanciprian/terraform-kubernetes-gke-cluster) to build a Private GKE Cluster for this purpose. Or use Kind or Docker-Desktop to build a local cluster
- **cert-manager**: Required for generating TLS certificates for the webhook and injecting caBundle in webhook configuration.
   - You can install cert-manager with [helm](https://artifacthub.io/packages/helm/cert-manager/cert-manager) or use my [flux config](https://github.com/andreistefanciprian/flux-demo/tree/main/infra/cert-manager).
- **Go**: The webhook is written in Go.
- **jq**: Used for parsing and manipulating JSON data in the Makefile.
- **Makefile**: The project uses a Makefile for automation and building. Understanding Makefile syntax will help you work with the provided build and deployment scripts.
- **Kustomize**: Used for bulding the test scenario manifests.

**Note**: In case you are using your own credentials for the container registry, make sure you set up these credentials as Github Secrets for your repo.
These credentials are used by Github Actions to push the image to dockerhub.

   ```
   # Set Github Actions secrets
   TOKEN=<dockerhub_auth_token>
   gh secret set DOCKERHUB_USERNAME -b"your_username"
   gh secret set DOCKERHUB_TOKEN -b"${TOKEN}"
   ```

**Note**: Make sure the priorityclass you want to configure for deployments exists in the cluster.

   ```
   kubectl apply -f https://raw.githubusercontent.com/andreistefanciprian/flux-demo/main/infra/priorityclasses/high-priority.yaml
   ```
## Build and Run the Webhook

Build, Register, Deploy and Test the webhook using the provided tasks:

1. Build and push the Docker image to the container registry:
   ```
   make unit-tests
   make build
   ```

2. Check webhook manifests that will be installed:
   ```
   make template
   ```

3. Deploy and Register webhook:
   **Note**: Also build a deployment before registering the webhook so we can test the Deployment UPDATE operation later.
   ```
   make install
   ```

4. Create test Deployments:
   ```
   # create Pods and Deployments
   make test
   ```

5. Verify Deployments were updated by webhook:
   ```
   # check webhook logs
   make logs

   # Test 1 - Checking that preexisting Deployment gets mutated by webhook
   kubectl patch deployment test-1 -n boo --type='json' -p='[{"op": "add", "path": "/metadata/annotations/patch", "value": "test"}]'
   kubectl patch deployment test-1 -n boo --type='json' -p='[{"op": "remove", "path": "/spec/template/spec/priorityClassName"}]'
   kubectl get deployment test-1 -n boo -o yaml --ignore-not-found | grep priority -A2 -B3

   # Test 2 - Checking that a Deployment without a priorityClassName gets mutated by webhook
   kubectl get deployment/test-2 -n boo -o yaml | grep priority -A2 -B3

   # Test 3 - Checking that a Deployment that has priorityClassName set gets mutated by webhook
   kubectl get deployment/test-3 -n boo -o yaml | grep priority -A2 -B3

   # Test 4 - Checking that a Deployment that has priorityClassName already set to high-priority-nonpreempting doesn't get mutated by webhook
   kubectl get deployment/test-4 -n boo -o yaml | grep priority -A2 -B3

   # Test 5 - Checking that a Pod without deployment doesn't get mutated by webhook
   kubectl get pod/pod -n boo -o yaml | grep priority -A2 -B3
   ```
   
6. Remove test resources and uninstall the webhook:
   ```
   make clean
   ```

Feel free to adjust the tasks and configurations as needed to fit your specific environment.

## License

This project is licensed under the [MIT License](LICENSE). Feel free to use and modify it according to your requirements.