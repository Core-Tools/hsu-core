@echo off
python -m grpc_tools.protoc -I. --python_out=../../python/lib/generated/api/proto --grpc_python_out=../../python/lib/generated/api/proto coreservice.proto
python ../../python/lib/generated/api/proto/fix_imports.py 