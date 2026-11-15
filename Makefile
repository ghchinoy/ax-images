# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Every directory under images/ is one image: images/<name>/Dockerfile is
# built with the repository root as context and pushed as
# $(IMAGE_REPO)/<name>:latest. The digest is written to bin/<name>.digest,
# and make pin writes it into the task.yaml and README.md.
#
#   make push-dgem-agent IMAGE_REPO=ghcr.io/ghchinoy/ax-images
#   make images IMAGE_REPO=ghcr.io/ghchinoy/ax-images
#   make pin IMAGE_REPO=ghcr.io/ghchinoy/ax-images
#   make apple-local        # build local arm64 images in macOS Apple container
#   make test

IMAGES := $(notdir $(patsubst %/,%,$(wildcard images/*/)))

IMAGE_REPO ?= ghcr.io/ghchinoy/ax-images
CONTAINER_CLI ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
BUILD_ARGS := $(if $(BASE_IMAGE),--build-arg BASE_IMAGE=$(BASE_IMAGE),)
ARCH ?= amd64
DGEM_REPO ?= $(HOME)/projects/dgem
AX_EXPERIMENTS_REPO ?= $(HOME)/projects/ax-experiments

.PHONY: binaries images pin list-images test apple-local sync check-image-repo $(addprefix build-,$(IMAGES)) $(addprefix push-,$(IMAGES))

binaries:
	@mkdir -p bin/linux_$(ARCH)
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/linux_$(ARCH)/ ./cmd/...

check-image-repo:
	@test -n "$(IMAGE_REPO)" || { echo "IMAGE_REPO is not set, e.g. IMAGE_REPO=ghcr.io/ghchinoy/ax-images"; exit 1; }

define image_rules
build-$(1): binaries check-image-repo
	$$(CONTAINER_CLI) build --platform linux/$$(ARCH) --build-arg TARGETARCH=$$(ARCH) $$(BUILD_ARGS) -t $$(IMAGE_REPO)/$(1):latest -f images/$(1)/Dockerfile .

push-$(1): build-$(1)
	@echo "==> Pushing $$(IMAGE_REPO)/$(1):latest..."
	@if $$(CONTAINER_CLI) --version 2>/dev/null | grep -qi podman; then \
		$$(CONTAINER_CLI) push --digestfile bin/$(1).digest $$(IMAGE_REPO)/$(1):latest; \
	else \
		$$(CONTAINER_CLI) push $$(IMAGE_REPO)/$(1):latest && \
		$$(CONTAINER_CLI) image inspect --format '{{range .RepoDigests}}{{println .}}{{end}}' $$(IMAGE_REPO)/$(1):latest | \
			grep "^$$(IMAGE_REPO)/$(1)@" | head -n1 | sed 's/.*@//' > bin/$(1).digest; \
	fi
	@echo "==> Pushed digest: $$(IMAGE_REPO)/$(1)@$$$$(cat bin/$(1).digest)"
endef
$(foreach img,$(IMAGES),$(eval $(call image_rules,$(img))))

images: $(addprefix push-,$(IMAGES))

apple-local:
	$(MAKE) binaries ARCH=arm64
	@for img in $(IMAGES); do \
		echo "==> Building Apple container image $(IMAGE_REPO)/$$img:latest (linux/arm64)..."; \
		container build --build-arg TARGETARCH=arm64 -t "$(IMAGE_REPO)/$$img:latest" -f "images/$$img/Dockerfile" .; \
	done

pin: check-image-repo
	@for img in $(IMAGES); do \
		test -f bin/$$img.digest || continue; \
		ref="$(IMAGE_REPO)/$$img@$$(cat bin/$$img.digest)"; \
		for f in images/$$img/task.yaml images/$$img/README.md; do \
			sed -i.bak -E "s|^([[:space:]]+image: ).*|\1\"$$ref\"|" $$f && rm $$f.bak; \
		done; \
		echo "$$img -> $$ref"; \
	done

list-images:
	@echo $(IMAGES)

sync:
	@echo "==> Syncing templates and safety-gate scripts from $(DGEM_REPO) and $(AX_EXPERIMENTS_REPO)..."
	@test -d "$(DGEM_REPO)" && { \
		cp "$(DGEM_REPO)"/templates/*.json.tmpl images/dgem-agent/templates/; \
		cp "$(DGEM_REPO)"/templates/calibration/*.json.tmpl images/dgem-agent/templates/calibration/; \
		cp "$(DGEM_REPO)"/examples/agy-safety-gate/dgem_gate.py "$(DGEM_REPO)"/examples/agy-safety-gate/sdk_demo.py images/agy-agent/agy-safety-gate/; \
	} || true
	@test -d "$(AX_EXPERIMENTS_REPO)" && { \
		cp "$(AX_EXPERIMENTS_REPO)/swarm/agent-image/agent.py" images/swarm-agent/agent.py; \
		cp "$(AX_EXPERIMENTS_REPO)/dgem/templates/specialist_router.json.tmpl" images/dgem-agent/templates/; \
	} || true

test:
	go vet ./...
	go test ./...
