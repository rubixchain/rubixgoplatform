from node.commands import cmd_shutdown_node
from node.actions import get_base_ports

quorum_base_server, quorum_grpc_server = get_base_ports()
offset = 4

for i in range(offset, 5 + offset):
    server_port = quorum_base_server + i
    grpc_port = quorum_grpc_server + i

    cmd_shutdown_node(server_port, grpc_port)

for i in range(offset, 2 + offset):
    server_port = quorum_base_server + 10 + i
    grpc_port = quorum_grpc_server + 10 + i

    cmd_shutdown_node(server_port, grpc_port)
