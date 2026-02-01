import ast
import json
import os
import re
import sys

MODULE_RE = re.compile(r"^(?P<name>(?!_)[a-z0-9_]+)(\.py)?$", re.I)
CLASS_RE = re.compile(r"^Notify(?!Base|ImageSize|Type)[A-Za-z0-9]+$")


def is_str_node(node):
    if isinstance(node, ast.Constant) and isinstance(node.value, str):
        return True
    if hasattr(ast, "Str") and isinstance(node, ast.Str):
        return True
    return False


def str_value(node):
    if isinstance(node, ast.Constant):
        return node.value
    return node.s


def eval_basic(node):
    if is_str_node(node):
        return str_value(node)
    if isinstance(node, (ast.List, ast.Tuple)):
        values = []
        for item in node.elts:
            value = eval_basic(item)
            if isinstance(value, str):
                values.append(value)
            elif isinstance(value, list):
                values.extend(value)
        return values
    if isinstance(node, ast.Dict):
        keys = []
        for key in node.keys:
            value = eval_basic(key)
            if isinstance(value, str):
                keys.append(value)
        return {k: None for k in keys}
    return None


def extract_constants(tree):
    constants = {}
    dict_constants = {}

    for node in tree.body:
        if not isinstance(node, ast.Assign):
            continue
        if len(node.targets) != 1:
            continue
        target = node.targets[0]
        if not isinstance(target, ast.Name):
            continue

        value = eval_basic(node.value)
        if isinstance(value, dict):
            dict_constants[target.id] = value
        elif isinstance(value, str):
            constants[target.id] = [value]
        elif isinstance(value, list):
            constants[target.id] = value

    return constants, dict_constants


def extract_constants_from_body(body):
    constants = {}
    dict_constants = {}

    for node in body:
        if not isinstance(node, ast.Assign):
            continue
        if len(node.targets) != 1:
            continue
        target = node.targets[0]
        if not isinstance(target, ast.Name):
            continue

        value = eval_basic(node.value)
        if isinstance(value, dict):
            dict_constants[target.id] = value
        elif isinstance(value, str):
            constants[target.id] = [value]
        elif isinstance(value, list):
            constants[target.id] = value

    return constants, dict_constants


def eval_protocol(node, constants, dict_constants, class_constants, class_dict_constants):
    if is_str_node(node):
        return [str_value(node)]
    if isinstance(node, (ast.List, ast.Tuple)):
        values = []
        for item in node.elts:
            value = eval_protocol(
                item, constants, dict_constants, class_constants, class_dict_constants
            )
            if value:
                values.extend(value)
        return values
    if isinstance(node, ast.Name):
        if node.id in class_constants:
            return class_constants[node.id]
        if node.id in class_dict_constants:
            return list(class_dict_constants[node.id].keys())
        if node.id in constants:
            return constants[node.id]
        if node.id in dict_constants:
            return list(dict_constants[node.id].keys())
    if isinstance(node, ast.Call):
        if isinstance(node.func, ast.Name) and node.func.id in ("list", "tuple", "set"):
            if len(node.args) == 1:
                return eval_protocol(
                    node.args[0], constants, dict_constants, class_constants, class_dict_constants
                )
        if isinstance(node.func, ast.Attribute) and node.func.attr == "keys":
            if isinstance(node.func.value, ast.Name):
                name = node.func.value.id
                if name in class_dict_constants:
                    return list(class_dict_constants[name].keys())
                if name in dict_constants:
                    return list(dict_constants[name].keys())
    return None


def extract_schemas(module_path):
    with open(module_path, "r", encoding="utf-8") as handle:
        tree = ast.parse(handle.read(), filename=module_path)

    constants, dict_constants = extract_constants(tree)
    schemas = set()

    for node in tree.body:
        if not isinstance(node, ast.ClassDef):
            continue
        if not CLASS_RE.match(node.name):
            continue

        class_constants, class_dict_constants = extract_constants_from_body(node.body)

        protocol_nodes = []
        for stmt in node.body:
            if isinstance(stmt, ast.Assign) and len(stmt.targets) == 1:
                target = stmt.targets[0]
                if not isinstance(target, ast.Name):
                    continue
                if target.id not in ("protocol", "secure_protocol"):
                    continue
                protocol_nodes.append(stmt.value)

        for value_node in protocol_nodes:
            values = eval_protocol(
                value_node, constants, dict_constants, class_constants, class_dict_constants
            )
            if not values:
                continue
            for value in values:
                if isinstance(value, str) and value:
                    schemas.add(value.lower())

    return schemas


def iter_modules(plugin_dir):
    for root, dirs, files in os.walk(plugin_dir):
        dirs[:] = [d for d in dirs if not d.startswith("__")]
        for filename in files:
            if not filename.endswith(".py"):
                continue
            if filename == "__init__.py":
                yield os.path.join(root, filename)
                continue
            if not MODULE_RE.match(filename):
                continue
            yield os.path.join(root, filename)


def main():
    if len(sys.argv) != 2:
        print("Usage: list_schemas.py /path/to/apprise", file=sys.stderr)
        sys.exit(2)

    source_root = sys.argv[1]
    plugin_dir = os.path.join(source_root, "apprise", "plugins")
    if not os.path.isdir(plugin_dir):
        print(f"Plugin dir not found: {plugin_dir}", file=sys.stderr)
        sys.exit(2)

    schemas = set()
    for module_path in iter_modules(plugin_dir):
        if not os.path.isfile(module_path):
            continue
        schemas.update(extract_schemas(module_path))

    print(json.dumps(sorted(schemas)))


if __name__ == "__main__":
    main()
