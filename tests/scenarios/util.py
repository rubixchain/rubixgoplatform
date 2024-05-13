def get_non_quorum_node_configs(config: dict):
    sender_config = {}
    receiver_config = {}
    
    for node in config:
        sender_config = config[node]
        receiver_config = config[node]

    return sender_config, receiver_config