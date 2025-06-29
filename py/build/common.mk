# Common Nuitka build components
# This file contains reusable build logic for Nuitka projects

# Detect OS and Shell Environment
ifeq ($(OS),Windows_NT)
    # Check if we're in MSYS/Git Bash environment
    ifneq ($(findstring msys,$(shell uname -s 2>/dev/null | tr A-Z a-z)),)
        # MSYS environment - use Unix-style commands
        DETECTED_OS := Windows-MSYS
        RM_RF := rm -rf
        MKDIR := mkdir -p
        COPY := cp
        PATH_SEP := /
        EXE_EXT := .exe
        NULL_DEV := /dev/null
    else
        # Native Windows
        DETECTED_OS := Windows
        RM_RF := rmdir /s /q
        MKDIR := mkdir
        COPY := copy
        PATH_SEP := \\
        EXE_EXT := .exe
        NULL_DEV := NUL
    endif
else
    DETECTED_OS := $(shell uname -s)
    RM_RF := rm -rf
    MKDIR := mkdir -p
    COPY := cp
    PATH_SEP := /
    EXE_EXT := 
    NULL_DEV := /dev/null
endif

# Common variables (can be overridden in project-specific Makefile)
PYTHON ?= python
BUILD_SUBDIR := $(OUTPUT_NAME)
BUILD_DIR := build$(PATH_SEP)$(BUILD_SUBDIR)
LOG_FILE := build-$(OUTPUT_NAME).log

# Executable names
ifeq ($(findstring Windows,$(DETECTED_OS)),Windows)
    BUILT_EXE := $(BUILD_DIR)$(PATH_SEP)$(OUTPUT_NAME).exe
else
    BUILT_EXE := $(BUILD_DIR)$(PATH_SEP)$(OUTPUT_NAME)
endif

# Read excluded packages from file
EXCLUDE_PACKAGES := $(shell grep -v '^\#' $(EXCLUDE_PACKAGES_FILE) | grep -v '^$$' | tr '\n' ' ')
NOFOLLOW_OPTS := $(foreach pkg,$(EXCLUDE_PACKAGES),--nofollow-import-to=$(pkg))

# Nuitka common options
NUITKA_COMMON_OPTS := \
    --assume-yes-for-downloads \
    --disable-ccache \
    --standalone \
    --show-progress \
    --show-memory \
    --include-package=grpc \
    --include-module=hsu_core.py.build.patch_meta \
    --include-module=hsu_core.py.api.proto.coreservice_pb2 \
    --include-module=hsu_core.py.api.proto.coreservice_pb2_grpc \
    --include-module=echocore.py.api.proto.echoservice_pb2 \
    --include-module=echocore.py.api.proto.echoservice_pb2_grpc \
    --include-module=jaraco.text \
    --include-package=tqdm \
    --include-package=yaml \
    $(NOFOLLOW_OPTS) \
    --include-module=importlib.metadata \
    --remove-output \
    --onefile \
    --follow-imports

# Platform-specific options
ifeq ($(findstring Windows,$(DETECTED_OS)),Windows)
    # Windows (both native and MSYS) - use exe extension and mingw64
    NUITKA_OPTS := $(NUITKA_COMMON_OPTS) --mingw64 --output-filename=$(OUTPUT_NAME).exe
else
    # Unix-like systems
    NUITKA_OPTS := $(NUITKA_COMMON_OPTS) --output-filename=$(OUTPUT_NAME)
endif

# Add output directory (use PATH_SEP for correct path format)
NUITKA_OPTS += --output-dir=build$(PATH_SEP)$(BUILD_SUBDIR)

# Common targets

# Build target
.PHONY: build
build:
	@echo "Building $(OUTPUT_NAME) with Nuitka on $(DETECTED_OS)..."
	@echo "Detected OS: $(DETECTED_OS)"
	
	# Clean previous build
	@if [ -d "$(BUILD_DIR)" ]; then \
		echo "Removing previous build directory..."; \
		$(RM_RF) "$(BUILD_DIR)" 2>$(NULL_DEV) || true; \
	fi
	
	# Run Nuitka build
	$(PYTHON) -m nuitka $(NUITKA_OPTS) $(SOURCE_FILE) > $(LOG_FILE) 2>&1 || (echo "Build failed! Check $(LOG_FILE) for details." && exit 1)
	
	@echo "Build completed. Check $(LOG_FILE) for details."
	
	# Check if build was successful
	@if [ -f "$(BUILT_EXE)" ]; then \
		echo "Build successful!"; \
		echo "Executable created at $(BUILT_EXE)"; \
	else \
		echo "Build failed! No executable was created."; \
		exit 1; \
	fi

# Clean target
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@$(RM_RF) "$(BUILD_DIR)" 2>$(NULL_DEV) || true
	@$(RM_RF) "$(LOG_FILE)" 2>$(NULL_DEV) || true
	@echo "Clean completed."

# Show configuration
.PHONY: config
config:
	@echo "Build Configuration:"
	@echo "  OS: $(DETECTED_OS)"
	@echo "  Python: $(PYTHON)"
	@echo "  Output Name: $(OUTPUT_NAME)"
	@echo "  Source File: $(SOURCE_FILE)"
	@echo "  Build Directory: $(BUILD_DIR)"
	@echo "  Built Executable: $(BUILT_EXE)"
	@echo "  Log File: $(LOG_FILE)"
	@echo "  Excluded Packages File: $(EXCLUDE_PACKAGES_FILE)"
	@echo "  Excluded Packages: $(EXCLUDE_PACKAGES)"

# Show excluded packages
.PHONY: show-excludes
show-excludes:
	@echo "Excluded packages from $(EXCLUDE_PACKAGES_FILE):"
	@echo "$(EXCLUDE_PACKAGES)" | tr ' ' '\n' | sort

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build         - Build the server with Nuitka (default)"
	@echo "  clean         - Remove build artifacts"
	@echo "  config        - Show build configuration"
	@echo "  show-excludes - Show excluded packages from $(EXCLUDE_PACKAGES_FILE)"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Project: $(OUTPUT_NAME)"
	@echo "Detected OS: $(DETECTED_OS)"
	@echo "Python command: $(PYTHON)" 