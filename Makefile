ROOT := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
ASSET_OUTPUT = /asset-output
LAMBDA_SRC = backend/pkg/*/*.go backend/pkg/*/*/*.go
LAMBDA_FUNCTIONS = build/bootstrap

lambda: $(LAMBDA_FUNCTIONS)

build/bootstrap: backend/lambda/*.go $(LAMBDA_SRC)
	go build -o build/bootstrap ./backend/lambda

FRONTEND_DIR = $(ROOT)/frontend

asset: lambda
	cp build/* $(ASSET_OUTPUT)
	cp -r $(FRONTEND_DIR)/dist/${STAGE} $(ASSET_OUTPUT)/assets
