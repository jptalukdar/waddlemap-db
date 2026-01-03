from setuptools import setup

setup(
    name="waddle-client",
    version="0.1.0",
    description="Python client for WaddleMap-DB",
    py_modules=["waddle_client", "waddle_protocol_pb2", "waddle_protocol_pb2_grpc"],
    install_requires=[
        "grpcio",
        "protobuf",
    ],
    python_requires='>=3.7',
)
