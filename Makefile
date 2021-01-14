SEVERITIES = HIGH,CRITICAL

.PHONY: all
all:
	docker build --build-arg TAG=$(TAG) -t rancher/gke-operator:$(TAG) .

.PHONY: image-push
image-push:
	docker push rancher/gke-operator:$(TAG) >> /dev/null

.PHONY: scan
image-scan:
	trivy --severity $(SEVERITIES) --no-progress --skip-update --ignore-unfixed rancher/gke-operator:$(TAG)

.PHONY: image-manifest
image-manifest:
	docker image inspect rancher/gke-operator:$(TAG)
	DOCKER_CLI_EXPERIMENTAL=enabled docker manifest create rancher/gke-operator:$(TAG) \
		$(shell docker image inspect rancher/gke-operator:$(TAG) | jq -r '.[] | .RepoDigests[0]')
