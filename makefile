SHELL := /bin/sh

.PHONY: start stop build check report

# Config
APP_NAME := fava-local
URL := http://localhost:5001
LEDGER := /ledger/main.beancount

EXPORTS_DIR := exports
QUERIES_DIR := queries

# Helper to run a .bql file through bean-query and output CSV.
# Usage: $(call BEANQUERY,<query-file>,<output-csv>)
define BEANQUERY
	@echo "▶ bean-query: $(1) -> $(2)"
	docker-compose run --rm \
		-e BEANCOUNT_FILE=$(LEDGER) \
		beancount sh -lc 'bean-query -f csv "$$BEANCOUNT_FILE" "$$(cat)"' \
		< $(1) \
		> $(2)
endef

## Start development environment
start: build
	@echo "▶ Starting Colima"
	colima start

	@echo "▶ Starting containers"
	docker-compose up -d

	@echo "▶ Opening browser"
	open $(URL)

## Build Docker image
build:
	@echo "▶ Building Docker image ($(APP_NAME))"
	docker build -t $(APP_NAME) .

## Stop development environment
stop:
	@echo "▶ Stopping containers"
	docker-compose down

## Check Beancount files (bean-check)
check:
	@echo "▶ Running bean-check"
	docker-compose --profile tools run --rm beancheck

## Generate reports (CSV) + convert to XLSX
report:
	@set -eu; \
	echo "▶ Generating reports"; \
	mkdir -p $(EXPORTS_DIR); \
	test -f "$(QUERIES_DIR)/income-select.bql"; \
	test -f "$(QUERIES_DIR)/expenses-select.bql"; \
	:
	$(call BEANQUERY,$(QUERIES_DIR)/income-select.bql,$(EXPORTS_DIR)/income-select.csv)
	$(call BEANQUERY,$(QUERIES_DIR)/expenses-select.bql,$(EXPORTS_DIR)/expenses-select.csv)

	@echo "▶ Converting CSV -> XLSX"
	docker-compose --profile tools run --rm csv2xlsx