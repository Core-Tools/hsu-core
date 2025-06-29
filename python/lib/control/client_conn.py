import grpc

class ClientConn():
    def __init__(self, port: int):
        self.grpc_client_connection = grpc.insecure_channel(f"localhost:{port}")

    def GRPC(self):
        if not self.grpc_client_connection:
            raise Exception("Client connection is not established")
        return self.grpc_client_connection

    def Shutdown(self):
        if self.grpc_client_connection:
            self.grpc_client_connection.close()

