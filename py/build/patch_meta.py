# Runtime hook to patch importlib.metadata for excluded packages
# Less aggressive approach - patch after normal import

import os
from pathlib import Path

def load_exclude_packages_and_versions(excludes_file, requirements_file):
    """Load package names from nuitka_excludes.txt and versions from requirements.txt"""
    
    # Load package names from nuitka_excludes.txt
    excludes_file = Path(excludes_file)
    excluded_packages = set()
    
    try:
        with open(excludes_file, 'r', encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#'):
                    excluded_packages.add(line)
    except FileNotFoundError:
        print(f"Warning: {excludes_file} not found")
    
    # Load versions from requirements.txt
    requirements_file = Path(requirements_file)
    package_versions = {}
    
    try:
        with open(requirements_file, 'r', encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#') and '==' in line:
                    package, version = line.split('==', 1)
                    package_versions[package.strip()] = version.strip()
    except FileNotFoundError:
        print(f"Warning: {requirements_file} not found")
    
    # Create fake versions dict with default version for packages without explicit version
    fake_versions = {}
    default_version = '1.0.0'
    
    for package in excluded_packages:
        # Try to get version from requirements.txt
        version = package_versions.get(package)
        if not version:
            # Handle special cases for package name variations
            if package == 'PIL':
                version = package_versions.get('pillow', default_version)
            elif package == 'sklearn':
                version = package_versions.get('scikit-learn', default_version)
            else:
                version = default_version
        
        fake_versions[package] = version
    
    # Also add special mappings for import vs package name differences
    if 'pillow' in package_versions:
        fake_versions['PIL'] = package_versions['pillow']
    if 'scikit-learn' in package_versions:
        fake_versions['sklearn'] = package_versions['scikit-learn']
    
    return fake_versions

def patch_importlib_metadata(excludes_file=None, requirements_file=None):
    """Patch importlib.metadata after it's imported normally"""
    
    # Load fake version information for excluded packages
    if excludes_file and requirements_file:
        FAKE_VERSIONS = load_exclude_packages_and_versions(excludes_file, requirements_file)
    else:
        FAKE_VERSIONS = {}
    
    try:
        import importlib.metadata as metadata
        
        # Store original functions
        original_version = metadata.version
        original_distribution = metadata.distribution
        
        def patched_version(dist_name):
            """Return fake version for excluded packages, real version for others"""
            if dist_name in FAKE_VERSIONS:
                return FAKE_VERSIONS[dist_name]
            
            # Handle name variations (e.g., scikit-learn vs sklearn)
            normalized_name = dist_name.lower().replace('-', '_')
            for fake_key in FAKE_VERSIONS:
                if fake_key.lower().replace('-', '_') == normalized_name:
                    return FAKE_VERSIONS[fake_key]
            
            # Try the original function
            try:
                return original_version(dist_name)
            except Exception:
                # If it's a package we know should be faked, return fake version
                raise  # Re-raise for truly missing packages
        
        def patched_distribution(dist_name):
            """Return fake distribution info for excluded packages"""
            if dist_name in FAKE_VERSIONS:
                return create_fake_distribution(dist_name, FAKE_VERSIONS[dist_name])
            
            # Handle name variations
            normalized_name = dist_name.lower().replace('-', '_')
            for fake_key in FAKE_VERSIONS:
                if fake_key.lower().replace('-', '_') == normalized_name:
                    return create_fake_distribution(dist_name, FAKE_VERSIONS[fake_key])
            
            # Try the original function
            return original_distribution(dist_name)
        
        # Apply patches
        metadata.version = patched_version
        metadata.distribution = patched_distribution
        
        return True
        
    except ImportError:
        # importlib.metadata not available
        return False

def create_fake_distribution(name, version):
    """Create a minimal fake distribution object"""
    class FakeDistribution:
        def __init__(self, name, version):
            self.metadata = {'Name': name, 'Version': version}
            self._name = name
            self._version = version
        
        @property 
        def name(self):
            return self._name
        
        @property
        def version(self):
            return self._version
            
        def __getitem__(self, key):
            return self.metadata.get(key)
    
    return FakeDistribution(name, version)

# Note: patch_importlib_metadata() must be called explicitly with file paths 