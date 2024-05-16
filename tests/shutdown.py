from node.commands import cmd_shutdown_node
from node.utils import get_base_ports
from config.utils import get_node_registry

if __name__=='__main__':
    base_node_server, base_grpc_server = get_base_ports()
    node_registry_config = get_node_registry()

    for indices in node_registry_config.values():
        for i in indices:
            server_port = base_node_server + i
            grpc_port = base_grpc_server + i
            print(f"Shutting down server running at {server_port}")
            cmd_shutdown_node(server_port, grpc_port)
