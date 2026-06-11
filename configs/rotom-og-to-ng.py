#!/usr/bin/env python3
"""
Rotom to RotomNG Config Converter

This script converts old Rotom JSON configuration files to the new Rotom TOML format.

Usage:
    python3 rotom-og-to-ng.py <input_json_file> [output_toml_file]

If output_toml_file is not specified, it will create a file with the same name but .toml extension.
"""

import json
import sys
import os
import re
from pathlib import Path
import argparse
from typing import Dict, Any, Optional


def strip_json_comments(json_string: str) -> str:
    """
    Remove JavaScript-style comments from JSON string.

    Args:
        json_string: JSON string that may contain comments

    Returns:
        JSON string with comments removed
    """
    # Remove single-line comments (// comment)
    # This regex matches // followed by anything until end of line
    json_string = re.sub(r'//.*?$', '', json_string, flags=re.MULTILINE)

    # Remove multi-line comments (/* comment */)
    json_string = re.sub(r'/\*.*?\*/', '', json_string, flags=re.DOTALL)

    return json_string


def convert_json_to_toml_config(json_config: Dict[str, Any]) -> Dict[str, Any]:
    """
    Convert Rotom JSON config to RotomNG TOML config structure.

    Args:
        json_config: The parsed JSON configuration

    Returns:
        Dictionary representing the TOML configuration structure
    """
    toml_config = {}

    # Convert device listener configuration
    if "deviceListener" in json_config:
        device_listener = json_config["deviceListener"]
        toml_config["device_listener"] = {}

        # Convert port to address format
        port = device_listener.get("port", 7070)
        toml_config["device_listener"]["address"] = f":{port}"

        # Add secret if present and not empty
        secret = device_listener.get("secret", "")
        if secret:
            toml_config["device_listener"]["secret"] = secret

    # Convert controller listener configuration
    if "controllerListener" in json_config:
        controller_listener = json_config["controllerListener"]
        toml_config["controller_listener"] = {}

        # Convert port to address format
        port = controller_listener.get("port", 7071)
        toml_config["controller_listener"]["address"] = f":{port}"

        # Add secret if present and not empty
        secret = controller_listener.get("secret", "")
        if secret:
            toml_config["controller_listener"]["secret"] = secret

    # Convert client/HTTP listener configuration
    if "client" in json_config:
        client = json_config["client"]
        toml_config["http_listener"] = {}

        # Convert host:port to address format
        host = client.get("host", "")
        port = client.get("port", 7072)
        if host and host != "0.0.0.0":
            toml_config["http_listener"]["address"] = f"{host}:{port}"
        else:
            toml_config["http_listener"]["address"] = f":{port}"

    # Convert logging configuration
    if "logging" in json_config:
        logging = json_config["logging"]
        toml_config["logging"] = {}

        # Map log level
        level = logging.get("level", "info")
        toml_config["logging"]["level"] = level

        # Map console status (inverted logic)
        console_status = logging.get("consoleStatus", False)
        toml_config["logging"]["no_console_log"] = not console_status

        # Set default format
        toml_config["logging"]["format"] = "plain"

        # Convert file logging if save is enabled
        if logging.get("save", False):
            toml_config["logging"]["file"] = {}
            toml_config["logging"]["file"]["disable"] = False
            toml_config["logging"]["file"]["path"] = "./logs/rotom.log"

            # Convert maxSize (MB) and maxAge (days)
            max_size = logging.get("maxSize", 100)
            toml_config["logging"]["file"]["max_size_mb"] = max_size

            max_age = logging.get("maxAge", 14)
            toml_config["logging"]["file"]["max_age_days"] = max_age

            # Set defaults for other file logging options
            toml_config["logging"]["file"]["max_backups"] = 30
            toml_config["logging"]["file"]["compress"] = False

    # Note: OG's "monitor" section (enabled, reboot, minMemory, maxMemStartMultiple,
    # maxMemStartMultipleOverwrite, deviceCooldown) has no equivalent in NG.
    # deviceCooldown was a per-device cooldown after controller allocation — this is
    # NOT the same as NG's rate_limit (which limits total selections per time window).
    # These settings are intentionally not converted.

    # Set default values for new config sections not present in old config
    toml_config.setdefault("jobs", {"enable": False, "path": "./jobs"})
    toml_config.setdefault("prometheus", {"enable": False})
    toml_config.setdefault("shutdown_timeout", "5s")

    return toml_config


def dict_to_toml_string(config: Dict[str, Any], indent: int = 0) -> str:
    """
    Convert a dictionary to TOML format string.

    Args:
        config: Dictionary to convert
        indent: Current indentation level

    Returns:
        TOML formatted string
    """
    lines = []
    indent_str = "  " * indent

    # First pass: handle non-dict values
    for key, value in config.items():
        if not isinstance(value, dict):
            if isinstance(value, str):
                lines.append(f'{indent_str}{key} = "{value}"')
            elif isinstance(value, bool):
                lines.append(f'{indent_str}{key} = {str(value).lower()}')
            elif isinstance(value, (int, float)):
                lines.append(f'{indent_str}{key} = {value}')

    # Second pass: handle dict values (sections)
    for key, value in config.items():
        if isinstance(value, dict):
            if lines:  # Add blank line before section if there are previous entries
                lines.append("")

            # Add section header
            if indent == 0:
                lines.append(f"[{key}]")
            else:
                lines.append(f"[{key}]")

            # Add section content
            section_content = dict_to_toml_string(value, indent)
            if section_content:
                lines.append(section_content)

    return "\n".join(lines)


def create_toml_config(config: Dict[str, Any]) -> str:
    """
    Create a properly formatted TOML configuration string.

    Args:
        config: Configuration dictionary

    Returns:
        Formatted TOML string
    """
    lines = [
        "# Rotom Configuration",
        "# Converted from Rotom JSON config",
        "#",
        "# All sections are optional - if not specified, sensible defaults will be used.",
        ""
    ]

    # Handle top-level sections in a specific order for better readability
    section_order = [
        "device_listener",
        "controller_listener",
        "http_listener",
        "rate_limit",
        "jobs",
        "logging",
        "prometheus"
    ]

    # Add sections in order
    for section_name in section_order:
        if section_name in config:
            lines.append(f"# {section_name.replace('_', ' ').title()} configuration")
            lines.append(f"[{section_name}]")

            section_config = config[section_name]
            for key, value in section_config.items():
                if isinstance(value, dict):
                    # Handle nested sections (like logging.file)
                    lines.append(f"[{section_name}.{key}]")
                    for nested_key, nested_value in value.items():
                        if isinstance(nested_value, str):
                            lines.append(f'{nested_key} = "{nested_value}"')
                        elif isinstance(nested_value, bool):
                            lines.append(f'{nested_key} = {str(nested_value).lower()}')
                        else:
                            lines.append(f'{nested_key} = {nested_value}')
                else:
                    if isinstance(value, str):
                        lines.append(f'{key} = "{value}"')
                    elif isinstance(value, bool):
                        lines.append(f'{key} = {str(value).lower()}')
                    else:
                        lines.append(f'{key} = {value}')
            lines.append("")

    # Add any remaining top-level settings
    for key, value in config.items():
        if key not in section_order:
            if isinstance(value, str):
                lines.append(f'{key} = "{value}"')
            elif isinstance(value, bool):
                lines.append(f'{key} = {str(value).lower()}')
            else:
                lines.append(f'{key} = {value}')

    return "\n".join(lines)


def main():
    """Main function to handle command line arguments and perform conversion."""
    parser = argparse.ArgumentParser(
        description="Convert Rotom JSON config to new Rotom TOML format"
    )
    parser.add_argument(
        "input_file",
        help="Path to the input JSON configuration file"
    )
    parser.add_argument(
        "output_file",
        nargs="?",
        help="Path to the output TOML configuration file (optional)"
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Overwrite output file if it exists"
    )

    args = parser.parse_args()

    # Validate input file
    input_path = Path(args.input_file)
    if not input_path.exists():
        print(f"Error: Input file '{args.input_file}' does not exist.", file=sys.stderr)
        sys.exit(1)

    if not input_path.is_file():
        print(f"Error: '{args.input_file}' is not a file.", file=sys.stderr)
        sys.exit(1)

    # Determine output file
    if args.output_file:
        output_path = Path(args.output_file)
    else:
        output_path = input_path.with_suffix('.toml')

    # Check if output file exists
    if output_path.exists() and not args.force:
        print(f"Error: Output file '{output_path}' already exists. Use --force to overwrite.", file=sys.stderr)
        sys.exit(1)

    try:
        # Read and parse JSON config
        with open(input_path, 'r', encoding='utf-8') as f:
            json_content = f.read()

        # Strip comments from JSON content
        json_content = strip_json_comments(json_content)

        # Parse the cleaned JSON
        json_config = json.loads(json_content)

        print(f"Successfully loaded JSON config from '{input_path}'")

        # Convert to TOML config structure
        toml_config = convert_json_to_toml_config(json_config)

        # Generate TOML string
        toml_content = create_toml_config(toml_config)

        # Write TOML config
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(toml_content)

        print(f"Successfully converted config to '{output_path}'")
        print("\nConversion Summary:")
        print(f"  - Device listener: port {json_config.get('deviceListener', {}).get('port', 'N/A')}")
        print(f"  - Controller listener: port {json_config.get('controllerListener', {}).get('port', 'N/A')}")
        client_config = json_config.get('client', {})
        host = client_config.get('host', '')
        port = client_config.get('port', 'N/A')
        if host and host != "0.0.0.0":
            print(f"  - HTTP listener: {host}:{port}")
        else:
            print(f"  - HTTP listener: port {port}")
        print(f"  - Logging level: {json_config.get('logging', {}).get('level', 'N/A')}")

        monitor_config = json_config.get('monitor', {})
        if monitor_config:
            dropped = [k for k in monitor_config if k in (
                'enabled', 'reboot', 'minMemory', 'maxMemStartMultiple',
                'maxMemStartMultipleOverwrite', 'deviceCooldown'
            ) and monitor_config[k] not in (0, False, {})]
            if dropped:
                print(f"\n  WARNING: The following OG 'monitor' settings have no NG equivalent and were dropped:")
                for key in dropped:
                    print(f"    - monitor.{key}: {monitor_config[key]}")

        print("\nNote: Please review the generated TOML file and adjust settings as needed.")
        print("If you were using jobs in OG, set 'enable = true' under [jobs] in the output file.")

    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON in input file: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
