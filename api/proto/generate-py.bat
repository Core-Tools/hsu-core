@echo off
python -m grpc_tools.protoc -I. --python_out=../../py/api/proto --grpc_python_out=../../py/api/proto coreservice.proto
python ../../py/api/proto/fix_imports.py 