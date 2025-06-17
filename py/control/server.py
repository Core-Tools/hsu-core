import sys
import grpc
import signal
from concurrent import futures

class Server:
    def __init__(self, port):
        # Create the server with a reasonable number of workers
        server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
        server_address = f'[::]:{port}'
        server.add_insecure_port(server_address)
        self.server_address = server_address
        self.server = server

    def GRPC(self):
        return self.server

    def run(self, on_shutdown_func):
        """Start the gRPC server on the specified port."""
        self.server.start()
        print(f"Server started, listening on {self.server_address}")
        print()
        
        # Setup signal handling for graceful shutdown
        def handle_shutdown_signal(signum, frame):
            print("\nReceived shutdown signal, stopping server...")
            # Stop accepting new requests
            self.server.stop(5)  # Wait up to 5 seconds for ongoing requests
            if on_shutdown_func is not None:
                on_shutdown_func()
            print("Server stopped.")
            sys.exit(0)
        
        # Register signal handlers
        signal.signal(signal.SIGINT, handle_shutdown_signal)  # Ctrl+C
        signal.signal(signal.SIGTERM, handle_shutdown_signal)  # kill command
        
        try:
            self.server.wait_for_termination()
        except KeyboardInterrupt:
            print()
            print("Server shutdown initiated...")
            self.server.stop(5)  # Wait up to 5 seconds for ongoing requests
            if on_shutdown_func is not None:
                on_shutdown_func()
            print("Server stopped.")
