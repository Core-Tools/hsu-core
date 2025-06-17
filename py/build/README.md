# Excluded Packages System

This document explains the refactored system for managing excluded packages in the Nuitka build process.

## Overview

The excluded packages system now uses centralized configuration files instead of hardcoded lists. This makes it easier to maintain and update the list of packages to exclude from the Nuitka build.

## Files

### `nuitka_excludes.txt`
Contains the list of package names to exclude from the Nuitka build, one per line. Comments (lines starting with `#`) are ignored.

**Usage:**
- Used by `Makefile` to generate `--nofollow-import-to` options
- Used by `patch_meta.py` to determine which packages need fake versions

### `requirements.txt`
Contains the packages and their versions in the standard `package==version` format. This file serves as the source of truth for package versions used in the fake metadata.

**Usage:**
- Used by `patch_meta.py` to load fake versions for excluded packages

### `patch_meta.py`
Runtime hook that patches `importlib.metadata` to provide fake version information for excluded packages.

**Changes made:**
- Now reads package names from `nuitka_excludes.txt`
- Now reads package versions from `requirements.txt`
- Includes fallback mechanisms if files are not found
- Handles package name variations (e.g., `PIL` vs `pillow`, `sklearn` vs `scikit-learn`)

### `Makefile`
Build script for creating the Nuitka executable.

**Changes made:**
- Reads excluded packages from `nuitka_excludes.txt` instead of hardcoded list
- Generates `--nofollow-import-to` options dynamically
- Added `show-excludes` target to display loaded packages
- Updated `config` and `help` targets

## Usage

### Adding a new package to exclude

1. Add the package name to `nuitka_excludes.txt`:
   ```
   new-package-name
   ```

2. Add the package with version to `requirements.txt`:
   ```
   new-package-name==1.0.0
   ```

### Viewing current excluded packages

```bash
make show-excludes
```

### Building with the new system

The build process remains the same:
```bash
make build
```

## Package Name Variations

The system handles common package name variations:

- **PIL vs pillow**: Both import names are supported, with `pillow` being the package name
- **sklearn vs scikit-learn**: Both names are supported, with `scikit-learn` being the package name

## Fallback Mechanism

If either `nuitka_excludes.txt` or `requirements.txt` is missing, the system falls back to the original hardcoded lists to ensure backward compatibility.

## Testing

To verify the system is working correctly:

1. Check that packages are loaded:
   ```bash
   python -c "import patch_meta; print('Loaded packages:', len(patch_meta.FAKE_VERSIONS))"
   ```

2. View the excluded packages:
   ```bash
   make show-excludes
   ```

3. Check build configuration:
   ```bash
   make config
   ```

## Benefits

- **Centralized configuration**: All excluded packages are managed in dedicated files
- **Easy maintenance**: Add/remove packages by editing text files
- **Version control friendly**: Changes are clearly visible in git diffs
- **Reusable**: The same files can be used by other tools if needed
- **Backward compatible**: Fallback mechanisms ensure existing functionality continues to work 