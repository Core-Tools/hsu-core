import sys
import os
import grpc

# Add the parent directory to sys.path to import the proto modules
parent_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.append(parent_dir)
# Also add the proto directory to path so coreservice_pb2_grpc can find coreservice_pb2
proto_dir = os.path.join(parent_dir, "generated", "api", "proto")
sys.path.append(proto_dir)

from ..generated.api.proto import coreservice_pb2
from ..generated.api.proto import coreservice_pb2_grpc
from ..domain.contract import Contract

def register_grpc_server_handler(grpc_server, handler: Contract):
    service = GRPCServerHandler(handler)
    coreservice_pb2_grpc.add_CoreServiceServicer_to_server(service, grpc_server)
    return service

def get_truncated_traceback(max_length=3800):
    import traceback

    traceback_str = traceback.format_exc()
    
    # Prevent excessive error message size (gRPC has a limit, typically 4KB)
    if len(traceback_str) > max_length:
        lines = traceback_str.splitlines()
        if len(lines) > 10:
            # Keep first 3 lines (traceback header) and last 7 lines (most relevant error)
            truncated_lines = lines[:3] + ["[...truncated...]"] + lines[-7:]
            traceback_str = "\n".join(truncated_lines)
    
    return traceback_str

class GRPCServerHandler(coreservice_pb2_grpc.CoreServiceServicer):
    """Implementation of the gRPC server service."""
    
    def __init__(self, handler):
        self.handler = handler
    
    def Ping(self, request, context):
        """Simple ping method."""
        try:
            self.handler.Ping()
            return coreservice_pb2.PingResponse()
        except Exception as e:
            self._handle_exception(e, context)
            return
            
    def _handle_exception(self, e, context):
        """Helper method to handle exceptions"""
        print(f"\n--- Exception --- ")
        traceback_str = get_truncated_traceback()
        print(traceback_str)
        print()
        
        context.set_code(grpc.StatusCode.INTERNAL)
        context.set_details(f"Exception: {traceback_str}")
        return True
        
    def __del__(self):
        """Clean up resources when the servicer is destroyed"""
        try:
            pass
        except:
            pass
