TF_IMAGE := hashicorp/terraform:1.16.4
IMAGE_TAG ?= $(shell git rev-parse --short=12 HEAD)

tf = docker run --rm -i -v "$(CURDIR)":/work -w /work/infra/$(1) -e DIGITALOCEAN_TOKEN -e TF_VAR_domain -e TF_VAR_github_token $(TF_IMAGE)

.PHONY: check up image smoke down

check:
	go vet ./...
	go test ./...
	docker run --rm -v "$(CURDIR)":/work -w /work $(TF_IMAGE) fmt -check -recursive infra
	for stage in cloud platform; do \
		$(call tf,$$stage) init -backend=false -input=false >/dev/null && $(call tf,$$stage) validate && $(call tf,$$stage) test || exit 1; \
	done

up:
	$(call tf,cloud) init -input=false
	$(call tf,cloud) apply
	$(MAKE) image
	$(call tf,platform) init -input=false
	$(call tf,platform) apply

image:
	@set -e; \
	config_dir=$$(mktemp -d); \
	trap 'rm -r "$$config_dir"' EXIT; \
	$(call tf,cloud) output -raw docker_config > "$$config_dir/config.json"; \
	ln -s "$$HOME/.docker/cli-plugins" "$$config_dir/cli-plugins"; \
	registry=$$($(call tf,cloud) output -raw registry_endpoint); \
	docker_host=$$(docker context inspect --format '{{.Endpoints.docker.Host}}'); \
	DOCKER_CONFIG="$$config_dir" DOCKER_HOST="$$docker_host" \
		docker buildx build --platform linux/amd64 -f build/Dockerfile -t "$$registry/minirail:$(IMAGE_TAG)" --metadata-file "$$config_dir/metadata.json" --push .; \
	digest=$$(sed -n 's/.*"containerimage.digest": *"\(sha256:[0-9a-f]*\)".*/\1/p' "$$config_dir/metadata.json"); \
	printf '{"image_digest": "%s"}\n' "$$digest" > infra/platform/image.auto.tfvars.json

smoke:
	CONSOLE_URL=$$($(call tf,platform) output -raw console_url) \
	ADMIN_PASSWORD=$$($(call tf,platform) output -raw admin_password) \
	APPS_DOMAIN=$$($(call tf,cloud) output -raw apps_domain) \
	scripts/smoke.sh

down:
	$(call tf,platform) destroy
	$(call tf,cloud) destroy
