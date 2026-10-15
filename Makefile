TF_IMAGE := hashicorp/terraform:1.16.4

tf = docker run --rm -i -v "$(CURDIR)":/work -w /work/infra/$(1) -e DIGITALOCEAN_TOKEN -e TF_VAR_domain -e TF_VAR_github_token -e TF_VAR_image_tag $(TF_IMAGE)

.PHONY: check

check:
	go vet ./...
	go test ./...
	docker run --rm -v "$(CURDIR)":/work -w /work $(TF_IMAGE) fmt -check -recursive infra
	for stage in cloud platform; do \
		$(call tf,$$stage) init -backend=false -input=false >/dev/null && $(call tf,$$stage) validate || exit 1; \
	done
