#
# Makefile for nrpe-exporter.
#
ARCHES := amd64 arm64
IMAGETAG=canonical/nrpe-exporter:latest
.PHONY: build docker-build
.DEFAULT_GOAL := build

## build: Create nrpe_exporter binaries
build: docker-build

## clean: Clean the cache and test caches
clean:
	@rm -f nrpe_exporter > /dev/null 2>&1 || true
	@docker rmi $(IMAGETAG) > /dev/null 2>&1 || true
	@docker image prune --filter="label=canonical=buildenv" --force > /dev/null 2>&1 || true


docker-build: $(ARCHES)

## docker-build: Build nrpe_exporter binaries without updating dependencies
$(ARCHES):
	@echo docker buildx build --platform linux/$@ --build-arg ARCH=$@ -t $(IMAGETAG) .
	@docker buildx build --platform linux/$@ --build-arg ARCH=$@ -t $(IMAGETAG) .
	@id=$$(docker create canonical/nrpe-exporter:latest) && \
	sudo docker cp $$id:/bin/nrpe_exporter ./nrpe_exporter-$@ && \
	(docker rm $$id > /dev/null)
	@docker rmi $(IMAGETAG) > /dev/null 2>&1
	@docker image prune --filter="label=canonical=buildenv" --force > /dev/null 2>&1
	@echo Finished: binary is at ./nrpe_exporter-$@
