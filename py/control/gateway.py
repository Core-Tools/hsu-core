from ..domain.contract import Contract
from ..api.proto import coreservice_pb2, coreservice_pb2_grpc

class Gateway(Contract):
    def __init__(self, channel):
        self.core_stub = coreservice_pb2_grpc.CoreServiceStub(channel)

    def Ping(self) -> str:
        self.core_stub.Ping(coreservice_pb2.PingRequest())
