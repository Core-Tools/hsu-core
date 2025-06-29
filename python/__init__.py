"""
HSU Core Library

HSU Repository Portability Framework - Core Components
Provides shared functionality for control, domain, and API layers.
"""

__version__ = "1.0.0"
__author__ = "HSU Core Team"

# Import key modules for easier access
from . import control
from . import domain
from . import api

# Make commonly used classes/functions available at package level
try:
    from .control.gateway import Gateway
    from .control.handler import Handler
    from .control.server import Server
    from .domain.contract import Contract
except ImportError:
    # Graceful degradation if specific modules aren't available
    pass

__all__ = [
    "control",
    "domain", 
    "api",
    "Gateway",
    "Handler",
    "Server",
    "Contract",
] 