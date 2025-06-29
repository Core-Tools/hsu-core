from .handler import register_grpc_server_handler
from ..domain.def_handler import DefaultHandler

def register_grpc_default_server_handler(grpc_server):
    register_grpc_server_handler(grpc_server, DefaultHandler())
