# -*- coding: utf-8 -*-
# Generated by the protocol buffer compiler.  DO NOT EDIT!
# NO CHECKED-IN PROTOBUF GENCODE
# source: coreservice.proto
# Protobuf Python Version: 5.29.0
"""Generated protocol buffer code."""
from google.protobuf import descriptor as _descriptor
from google.protobuf import descriptor_pool as _descriptor_pool
from google.protobuf import runtime_version as _runtime_version
from google.protobuf import symbol_database as _symbol_database
from google.protobuf.internal import builder as _builder
_runtime_version.ValidateProtobufRuntimeVersion(
    _runtime_version.Domain.PUBLIC,
    5,
    29,
    0,
    '',
    'coreservice.proto'
)
# @@protoc_insertion_point(imports)

_sym_db = _symbol_database.Default()




DESCRIPTOR = _descriptor_pool.Default().AddSerializedFile(b'\n\x11\x63oreservice.proto\x12\x05proto\"\r\n\x0bPingRequest\"\x0e\n\x0cPingResponse2@\n\x0b\x43oreService\x12\x31\n\x04Ping\x12\x12.proto.PingRequest\x1a\x13.proto.PingResponse\"\x00\x42*Z(github.com/core-tools/hsu-core/api/protob\x06proto3')

_globals = globals()
_builder.BuildMessageAndEnumDescriptors(DESCRIPTOR, _globals)
_builder.BuildTopDescriptorsAndMessages(DESCRIPTOR, 'coreservice_pb2', _globals)
if not _descriptor._USE_C_DESCRIPTORS:
  _globals['DESCRIPTOR']._loaded_options = None
  _globals['DESCRIPTOR']._serialized_options = b'Z(github.com/core-tools/hsu-core/api/proto'
  _globals['_PINGREQUEST']._serialized_start=28
  _globals['_PINGREQUEST']._serialized_end=41
  _globals['_PINGRESPONSE']._serialized_start=43
  _globals['_PINGRESPONSE']._serialized_end=57
  _globals['_CORESERVICE']._serialized_start=59
  _globals['_CORESERVICE']._serialized_end=123
# @@protoc_insertion_point(module_scope)
