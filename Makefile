.DEFAULT_GOAL := help
.PHONY: help onboard docs serve test

help:
	@echo "make onboard  Check access to the assigned workshop environment."
	@echo "make docs     Build the workshop guide."
	@echo "make serve    Build and serve the guide on http://localhost:8443."
	@echo "make test     Run local regression checks."

onboard:
	bash scripts/onboard.sh

docs:
	npx --yes --package @antora/cli@3.1 --package @antora/site-generator@3.1 antora generate default-site.yml --clean
	touch www/.nojekyll
	mkdir -p www/docs && cp docs/index.html www/docs/index.html

serve: docs
	python3 -m http.server 8443 --bind 127.0.0.1 --directory www

test:
	python3 -m unittest discover -s e2e -p 'test_*.py'
	cd examples/live-transcript && go test -race ./...
	cd examples/decision-tetris && go test -race ./...
	cd examples/orbit-checkpoint && go test -race ./...
	cd examples/sky-control && go test -race ./...
