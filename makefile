DOCKER_HUB_USERNAME := andreistefanciprian
IMAGE_NAME := k8s-priorityclass-webhook
DOCKER_IMAGE_NAME := $(DOCKER_HUB_USERNAME)/$(IMAGE_NAME)

build:
	docker build -t $(DOCKER_IMAGE_NAME) . -f infra/Dockerfile
	docker image push $(DOCKER_IMAGE_NAME)

template:
	helm template --namespace priorityclass-webhook priorityclass-webhook infra/priorityclass-webhook --create-namespace

install: test-pre-deployment
	sleep 5
	helm upgrade --install priorityclass-webhook infra/priorityclass-webhook --namespace priorityclass-webhook --create-namespace

uninstall:
	helm uninstall priorityclass-webhook --namespace priorityclass-webhook

test: test-post-deployment

test-post-deployment:
	@echo Builds test Deployments after webhook registration...
	kustomize build infra/test-create | kubectl apply -f -

test-pre-deployment:
	@echo Builds test Deployment before webhook registration...
	kustomize build infra/test-update | kubectl apply -f -
	
clean-tests:
	kustomize build infra/test-create | kubectl delete --ignore-not-found=true -f -
	kustomize build infra/test-update | kubectl delete --ignore-not-found=true -f -

clean: uninstall clean-tests
	kubectl delete ns priorityclass-webhook --ignore-not-found=true

check:
	helm list --namespace priorityclass-webhook
	kubectl get MutatingWebhookConfiguration priorityclass-webhook --ignore-not-found=true -n priorityclass-webhook
	kubectl get pods,secrets,certificates -n priorityclass-webhook

logs:
	kubectl logs -l app.kubernetes.io/name=priorityclass-webhook --namespace priorityclass-webhook -f

unit-tests:
	go test  ./... -v