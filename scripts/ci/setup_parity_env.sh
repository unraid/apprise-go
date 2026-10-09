#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."

python3 -m venv .venv
. .venv/bin/activate

python -m pip install --upgrade pip
if [[ -f ../apprise/pyproject.toml ]]; then
  python -m pip install -e "../apprise[all-plugins]"
else
  python -m pip install "apprise[all-plugins]"
fi

# Forward-port plugins not yet released in tagged Apprise (kept in
# internal/parity/upstream_plugins/). Drop once the matching upstream
# Apprise release includes them.
plugin_src="internal/parity/upstream_plugins/wpush.py"
if [[ -f "${plugin_src}" ]]; then
  plugin_dst="$(python -c 'import apprise, pathlib; print(pathlib.Path(apprise.__file__).resolve().parent / "plugins")')"
  cp "${plugin_src}" "${plugin_dst}/wpush.py"
  echo "Installed forward-port plugin: ${plugin_dst}/wpush.py"
fi
