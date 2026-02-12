SHELL      := /bin/bash
.DEFAULT_GOAL := help

VERSION    ?= dev
BUILD_DIR  := build
SRC_DIR    := src
TPL_DIR    := templates
OUTPUT     := $(BUILD_DIR)/vibrate.sh
INTER_DIR  := $(BUILD_DIR)/.intermediate

# Cross-platform helpers
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  B64_ENC := base64
else
  B64_ENC := base64 -w0
endif
# Portable sed-in-place: write to tmp then move back (avoids macOS sed -i '' issues)
define sed_inplace
	sed $(1) $(2) > $(2).tmp && mv $(2).tmp $(2)
endef

# Source modules in dependency order (header must be first, main must be last)
MODULES := \
	$(SRC_DIR)/header.sh \
	$(SRC_DIR)/lib/logging.sh \
	$(SRC_DIR)/lib/checks.sh \
	$(SRC_DIR)/lib/config.sh \
	$(SRC_DIR)/lib/args.sh \
	$(SRC_DIR)/lib/docker_cmd.sh \
	$(SRC_DIR)/lib/image.sh \
	$(SRC_DIR)/lib/container.sh \
	$(SRC_DIR)/lib/plugins.sh \
	$(SRC_DIR)/lib/dockerfile.sh \
	$(SRC_DIR)/main.sh

# Template scripts to base64-encode and embed
EMBED_SCRIPTS := entrypoint.sh claude-exec.sh zshrc setup-plugins.sh

# Container rules files
CONTAINER_RULES := $(TPL_DIR)/container-rules/docker-container-context.md $(TPL_DIR)/container-rules/safety-restrictions.md

# All template files (for dependency tracking)
TPL_FILES := $(TPL_DIR)/Dockerfile.template $(addprefix $(TPL_DIR)/,$(EMBED_SCRIPTS)) $(CONTAINER_RULES)

.PHONY: build clean lint validate help

build: $(OUTPUT)  ## Build vibrate.sh (use VERSION=x.y.z)

$(OUTPUT): $(MODULES) $(TPL_FILES) Makefile
	@mkdir -p $(BUILD_DIR) $(INTER_DIR)
	@echo "Building vibrate.sh version $(VERSION)..."
	@# Step 1: Base64-encode each embedded script
	@for script in $(EMBED_SCRIPTS); do \
		if [ -f "$(TPL_DIR)/$$script" ]; then \
			$(B64_ENC) < "$(TPL_DIR)/$$script" > "$(INTER_DIR)/$${script}.b64"; \
		else \
			echo "WARNING: $(TPL_DIR)/$$script not found, using empty placeholder" >&2; \
			echo -n "" | $(B64_ENC) > "$(INTER_DIR)/$${script}.b64"; \
		fi; \
	done
	@# Step 2: Base64-encode the Dockerfile template
	@if [ -f "$(TPL_DIR)/Dockerfile.template" ]; then \
		$(B64_ENC) < "$(TPL_DIR)/Dockerfile.template" > "$(INTER_DIR)/Dockerfile.b64"; \
	else \
		echo "WARNING: Dockerfile.template not found" >&2; \
		echo -n "" | $(B64_ENC) > "$(INTER_DIR)/Dockerfile.b64"; \
	fi
	@# Step 2b: Base64-encode container rules
	@if [ -f "$(TPL_DIR)/container-rules/docker-container-context.md" ]; then \
		$(B64_ENC) < "$(TPL_DIR)/container-rules/docker-container-context.md" > "$(INTER_DIR)/container-rules-context.b64"; \
	else \
		echo "WARNING: container-rules/docker-container-context.md not found" >&2; \
		echo -n "" | $(B64_ENC) > "$(INTER_DIR)/container-rules-context.b64"; \
	fi
	@if [ -f "$(TPL_DIR)/container-rules/safety-restrictions.md" ]; then \
		$(B64_ENC) < "$(TPL_DIR)/container-rules/safety-restrictions.md" > "$(INTER_DIR)/container-rules-safety.b64"; \
	else \
		echo "WARNING: container-rules/safety-restrictions.md not found" >&2; \
		echo -n "" | $(B64_ENC) > "$(INTER_DIR)/container-rules-safety.b64"; \
	fi
	@# Step 3: Concatenate source modules
	@# Keep shebang from header.sh, strip shebangs from all others
	@head -1 $(SRC_DIR)/header.sh > $(OUTPUT)
	@for mod in $(MODULES); do \
		echo "" >> $(OUTPUT); \
		echo "# --- $$(basename $$mod) ---" >> $(OUTPUT); \
		tail -n +2 "$$mod" | grep -v '^#!/' >> $(OUTPUT); \
	done
	@# Step 4: Replace version placeholder
	@$(call sed_inplace,'s/%%VERSION%%/$(VERSION)/g',$(OUTPUT))
	@# Step 5: Replace template placeholders with base64 content
	@$(call sed_inplace,"s|%%ENTRYPOINT_B64%%|$$(cat $(INTER_DIR)/entrypoint.sh.b64)|g",$(OUTPUT))
	@$(call sed_inplace,"s|%%CLAUDE_EXEC_B64%%|$$(cat $(INTER_DIR)/claude-exec.sh.b64)|g",$(OUTPUT))
	@$(call sed_inplace,"s|%%ZSHRC_B64%%|$$(cat $(INTER_DIR)/zshrc.b64)|g",$(OUTPUT))
	@$(call sed_inplace,"s|%%SETUP_PLUGINS_B64%%|$$(cat $(INTER_DIR)/setup-plugins.sh.b64)|g",$(OUTPUT))
	@$(call sed_inplace,"s|%%DOCKERFILE_TPL_B64%%|$$(cat $(INTER_DIR)/Dockerfile.b64)|g",$(OUTPUT))
	@$(call sed_inplace,"s|%%CONTAINER_RULES_CONTEXT_B64%%|$$(cat $(INTER_DIR)/container-rules-context.b64)|g",$(OUTPUT))
	@$(call sed_inplace,"s|%%CONTAINER_RULES_SAFETY_B64%%|$$(cat $(INTER_DIR)/container-rules-safety.b64)|g",$(OUTPUT))
	@# Step 6: Make executable and clean up
	@chmod +x $(OUTPUT)
	@rm -rf $(INTER_DIR)
	@echo "Built: $(OUTPUT) ($$(wc -l < $(OUTPUT)) lines)"

clean:  ## Remove build artifacts
	rm -rf $(BUILD_DIR)

lint:  ## Run shellcheck on source files
	@echo "Linting source modules..."
	@shellcheck -s bash $(MODULES) || true
	@echo "Linting template scripts..."
	@for f in $(TPL_DIR)/entrypoint.sh $(TPL_DIR)/claude-exec.sh; do \
		[ -f "$$f" ] && shellcheck -s bash "$$f" || true; \
	done
	@echo "Lint complete."

validate: build  ## Build and validate output
	@echo "Validating $(OUTPUT)..."
	@bash -n $(OUTPUT) && echo "  Syntax:  OK" || (echo "  Syntax:  FAIL" && exit 1)
	@$(OUTPUT) --version 2>/dev/null | grep -q "$(VERSION)" \
		&& echo "  Version: OK" \
		|| echo "  Version: WARN (mismatch)"
	@$(OUTPUT) --help > /dev/null 2>&1 \
		&& echo "  Help:    OK" \
		|| (echo "  Help:    FAIL" && exit 1)
	@echo "Validation complete."

help:  ## Show available targets
	@echo "vibrator build system"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*##"}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Usage: make build VERSION=1.2.3"
