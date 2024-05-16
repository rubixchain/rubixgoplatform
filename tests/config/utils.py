import json
import os

def get_node_registry():
    current_dir = os.path.dirname(os.path.abspath(__file__))
    node_registry_path = os.path.join(current_dir, "node_registry.json")
    config = load_from_config_file(node_registry_path)

    if config == {}:
        raise Exception("node registry config is empty")
    return config

def load_from_config_file(config_file_path):
    try:
        with open(config_file_path, 'r') as file:
            config_data = json.load(file)
        return config_data
    except FileNotFoundError as e:
        return {}
    except json.JSONDecodeError as e:
        raise ValueError(f"Error: The file at {config_file_path} is not a valid JSON file.") from e
    except Exception as e:
        raise Exception(f"An unexpected error occurred: {e}") from e

def save_to_config_file(config_file_path, config):
    try:
        if os.path.exists(config_file_path):
            os.remove(config_file_path)
        
        with open(config_file_path, 'w') as f:
            json.dump(config, f, indent=4)
    except FileNotFoundError as e:
        raise FileNotFoundError(f"Error: The file at {config_file_path} could not be found.") from e
    except PermissionError as e:
        raise PermissionError(f"Error: Permission denied when trying to write to {config_file_path}.") from e
    except TypeError as e:  # JSON serialization errors raise TypeError, not JSONDecodeError
        raise TypeError(f"Error: Failed to serialize the config data to JSON.") from e
    except Exception as e:
        raise Exception(f"An unexpected error occurred: {e}") from e
