#
# Makefile for nrpe-exporter.
#

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

## docker-build: Build nrpe_exporter binaries without updating dependencies
docker-build:
	@docker build -t $(IMAGETAG) .
	@id=$$(docker create canonical/nrpe-exporter:latest) && \
      sudo docker cp $$id:/bin/nrpe_exporter . && \
	  (docker rm $$id > /dev/null)
	@docker rmi $(IMAGETAG) > /dev/null 2>&1
	@docker image prune --filter="label=canonical=buildenv" --force > /dev/null 2>&1
	@echo Finished: binary is at ./nrpe_exporter
