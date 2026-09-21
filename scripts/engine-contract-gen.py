#!/usr/bin/env python3
"""Minimal third-party re-implementation of the engine-manifest-artifact-gen
splice for LunaFox Engine API v2 extensions.

Reads one strict engine.v5 engine.json and emits:
  - contract/execution_generated.go      (full typed contract; Config block
                                          is regenerated from configSections)
  - cmd/<binary>/execution_run_generated.go (entry adapter; config projection
                                          functions regenerated)

Everything except the Config-derived code is copied verbatim from the
directory_scan reference engine, which is byte-identical to every other
first-party engine outside those regions (verified by diff against port_scan).

Usage: python3 scripts/engine-contract-gen.py extensions/engines/<name>
"""
import json
import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TEMPLATE_ENGINE = "directory_scan"

GO_TYPES = {"boolean": "bool", "integer": "int64", "string": "string", "stringArray": "[]string"}
PROTO_WRAPPERS = {
    "boolean": ("ConfigValue_BooleanValue", "BooleanValue"),
    "integer": ("ConfigValue_IntegerValue", "IntegerValue"),
    "string": ("ConfigValue_StringValue", "StringValue"),
    "stringArray": ("ConfigValue_StringArrayValue", "StringArrayValue"),
}


def camel(key: str) -> str:
    return "".join(part[:1].upper() + part[1:] for part in re.split(r"[-_]", key))


def go_field(key: str) -> str:
    # match the upstream generator: first letter upper, kebab parts joined capitalized
    parts = key.split("-")
    return parts[0][:1].upper() + parts[0][1:] + "".join(p[:1].upper() + p[1:] for p in parts[1:])


def scalar_params(section):
    return [p for p in section.get("params", []) if "resource" not in p]


def resource_params(section):
    return [p for p in section.get("params", []) if "resource" in p]


def gen_contract_config(manifest):
    lines = ["type Config struct {"]
    for section in manifest["execution"]["configSections"]:
        lines.append(f'\t// Source: engine.json.execution.configSections["{section["id"]}"].')
        lines.append(f"\t{camel(section['id'])} {camel(section['id'])}Config")
    lines.append("}")
    lines.append("")
    for section in manifest["execution"]["configSections"]:
        sid = section["id"]
        lines.append(f"type {camel(sid)}Config struct {{")
        lines.append(f'\t// Source: engine.json.execution.configSections["{sid}"].defaultEnabled.')
        lines.append("\tEnabled bool")
        for param in section.get("params", []):
            key = param["key"]
            src = f'\t// Source: engine.json.execution.configSections["{sid}"].params["{key}"].'
            lines.append(src)
            if "resource" in param:
                lines.append(f"\t{go_field(key)} FilePath")
            else:
                lines.append(f"\t{go_field(key)} {GO_TYPES[param['type']]}")
        lines.append("}")
        lines.append("")
    return "\n".join(lines).rstrip("\n") + "\n"


def gen_section_projection(section):
    sid = section["id"]
    struct_t = f"enginecontract.{camel(sid)}Config"
    fn = f"lunafoxProject{camel(sid)}Config"
    scalars = scalar_params(section)
    body = []
    body.append(f"func {fn}(section *engineexecutionpb.ConfigSection) ({struct_t}, error) {{")
    body.append("\tif section.Enabled == nil {")
    body.append(f'\t\treturn {struct_t}{{}}, errors.New("Engine config enabled presence is required")')
    body.append("\t}")
    body.append(f"\tprojected := {struct_t}{{Enabled: section.GetEnabled()}}")
    body.append("\tif !section.GetEnabled() {")
    body.append("\t\tif len(section.GetParams()) != 0 {")
    body.append(f'\t\t\treturn {struct_t}{{}}, errors.New("disabled Engine config section must not contain scalar parameters")')
    body.append("\t\t}")
    body.append("\t\treturn projected, nil")
    body.append("\t}")
    for param in scalars:
        body.append(f"\tvar seen{go_field(param['key'])} bool")
    if scalars:
        body.append("\tfor _, param := range section.GetParams() {")
        body.append("\t\tif param == nil || param.GetValue() == nil {")
        body.append(f'\t\t\treturn {struct_t}{{}}, errors.New("Engine config scalar value is required")')
        body.append("\t\t}")
        body.append("\t\tswitch param.GetParamKey() {")
        for param in scalars:
            key = param["key"]
            field = go_field(key)
            wrapper, getter = PROTO_WRAPPERS[param["type"]]
            body.append(f'\t\tcase "{key}":')
            body.append(f"\t\t\tif seen{field} {{")
            body.append(f'\t\t\t\treturn {struct_t}{{}}, errors.New("duplicate Engine config scalar parameter")')
            body.append("\t\t\t}")
            body.append(f"\t\t\tseen{field} = true")
            body.append(f"\t\t\ttyped, ok := param.GetValue().(*engineexecutionpb.{wrapper})")
            body.append("\t\t\tif !ok {")
            body.append(f'\t\t\t\treturn {struct_t}{{}}, errors.New("Engine config scalar type does not match the Engine Definition")')
            body.append("\t\t\t}")
            if param["type"] == "stringArray":
                body.append(f"\t\t\tprojected.{field} = typed.{getter}.Values")
            else:
                body.append(f"\t\t\tprojected.{field} = typed.{getter}")
        body.append("\t\tdefault:")
        body.append(f'\t\t\treturn {struct_t}{{}}, errors.New("Engine config contains an undeclared scalar parameter")')
        body.append("\t\t}")
        body.append("\t}")
    else:
        body.append("\tif len(section.GetParams()) != 0 {")
        body.append(f'\t\treturn {struct_t}{{}}, errors.New("Engine config contains an undeclared scalar parameter")')
        body.append("\t}")
    for param in scalars:
        body.append(f"\tif !seen{go_field(param['key'])} {{")
        body.append(f'\t\treturn {struct_t}{{}}, errors.New("Engine config is missing a required scalar parameter")')
        body.append("\t}")
    body.append("\treturn projected, nil")
    body.append("}")
    return "\n".join(body)


def gen_config_projection(manifest):
    sections = manifest["execution"]["configSections"]
    body = []
    body.append("func lunafoxProjectConfig(config *engineexecutionpb.EngineExecutionConfig) (enginecontract.Config, error) {")
    body.append(f"\tif config == nil || len(config.GetSections()) != {len(sections)} {{")
    body.append('\t\treturn enginecontract.Config{}, errors.New("Engine config sections do not match the Engine Definition")')
    body.append("\t}")
    body.append("\tvar projected enginecontract.Config")
    for section in sections:
        body.append(f"\tvar seen{camel(section['id'])} bool")
    body.append("\tfor _, section := range config.GetSections() {")
    body.append("\t\tif section == nil {")
    body.append('\t\t\treturn enginecontract.Config{}, errors.New("Engine config section is required")')
    body.append("\t\t}")
    body.append("\t\tswitch section.GetSectionId() {")
    for section in sections:
        sid = section["id"]
        body.append(f'\t\tcase "{sid}":')
        body.append(f"\t\t\tif seen{camel(sid)} {{")
        body.append('\t\t\t\treturn enginecontract.Config{}, errors.New("duplicate Engine config section")')
        body.append("\t\t\t}")
        body.append(f"\t\t\tseen{camel(sid)} = true")
        body.append(f"\t\t\tvalue, err := lunafoxProject{camel(sid)}Config(section)")
        body.append("\t\t\tif err != nil {")
        body.append("\t\t\t\treturn enginecontract.Config{}, err")
        body.append("\t\t\t}")
        body.append(f"\t\t\tprojected.{camel(sid)} = value")
    body.append("\t\tdefault:")
    body.append('\t\t\treturn enginecontract.Config{}, errors.New("Engine config contains an undeclared section")')
    body.append("\t\t}")
    body.append("\t}")
    for section in sections:
        body.append(f"\tif !seen{camel(section['id'])} {{")
        body.append('\t\treturn enginecontract.Config{}, errors.New("Engine config is missing a declared section")')
        body.append("\t}")
    body.append("\treturn projected, nil")
    body.append("}")
    return "\n".join(body)


def gen_config_resources(manifest):
    sections = manifest["execution"]["configSections"]
    body = []
    body.append("func lunafoxProjectConfigResources(resources []*engineexecutionpb.ConfigResource, config *enginecontract.Config) error {")
    body.append("\tif config == nil {")
    body.append('\t\treturn errors.New("Engine config resources do not match the Engine Definition")')
    body.append("\t}")
    for section in sections:
        sid = section["id"]
        for param in resource_params(section):
            body.append(f"\tvar seen{camel(sid)}{go_field(param['key'])} bool")
    body.append("\tfor _, resource := range resources {")
    body.append("\t\tif resource == nil || resource.GetPath() == \"\" || resource.GetContentType() == \"\" {")
    body.append('\t\t\treturn errors.New("complete Engine config resource binding is required")')
    body.append("\t\t}")
    body.append("\t\tswitch {")
    for section in sections:
        sid = section["id"]
        for param in resource_params(section):
            key = param["key"]
            body.append(f'\t\tcase resource.GetSectionId() == "{sid}" && resource.GetParamKey() == "{key}":')
            body.append(f"\t\t\tif !config.{camel(sid)}.Enabled {{")
            body.append('\t\t\t\treturn errors.New("Engine config resource binding is forbidden for a disabled section")')
            body.append("\t\t\t}")
            body.append(f"\t\t\tif seen{camel(sid)}{go_field(key)} {{")
            body.append('\t\t\t\treturn errors.New("duplicate Engine config resource binding")')
            body.append("\t\t\t}")
            body.append(f"\t\t\tseen{camel(sid)}{go_field(key)} = true")
            body.append(f"\t\t\tconfig.{camel(sid)}.{go_field(key)} = resource.GetPath()")
    body.append("\t\tdefault:")
    body.append('\t\t\treturn errors.New("Engine config contains an undeclared resource binding")')
    body.append("\t\t}")
    body.append("\t}")
    for section in sections:
        sid = section["id"]
        for param in resource_params(section):
            key = param["key"]
            body.append(f"\tif config.{camel(sid)}.Enabled && !seen{camel(sid)}{go_field(key)} {{")
            body.append('\t\t\treturn errors.New("Engine config is missing a declared resource binding")')
            body.append("\t}")
    body.append("\treturn nil")
    body.append("}")
    return "\n".join(body)


def main():
    if len(sys.argv) != 2:
        print(__doc__)
        sys.exit(2)
    engine_dir = sys.argv[1]
    name = os.path.basename(os.path.normpath(engine_dir))
    with open(os.path.join(engine_dir, "engine.json"), encoding="utf-8") as handle:
        manifest = json.load(handle)

    template_root = os.path.join(REPO, "extensions", "engines", TEMPLATE_ENGINE)

    # contract
    with open(os.path.join(template_root, "contract", "execution_generated.go"), encoding="utf-8") as handle:
        contract = handle.read()
    start = contract.index("type Config struct {")
    end = contract.index("type PlatformResources struct {")
    contract = contract[:start] + gen_contract_config(manifest) + "\n" + contract[end:]
    out_dir = os.path.join(engine_dir, "contract")
    os.makedirs(out_dir, exist_ok=True)
    with open(os.path.join(out_dir, "execution_generated.go"), "w", encoding="utf-8") as handle:
        handle.write(contract)

    # run adapter
    with open(os.path.join(template_root, "cmd", f"{TEMPLATE_ENGINE.replace('_', '-')}-engine", "execution_run_generated.go"), encoding="utf-8") as handle:
        adapter = handle.read()
    adapter = adapter.replace(f"engines/{TEMPLATE_ENGINE}/contract", f"engines/{name}/contract")
    start = adapter.index("func lunafoxProjectConfig(")
    end = adapter.index("func lunafoxProjectPlatformResources(")
    mid = "\n\n".join(
        [gen_config_projection(manifest)]
        + [gen_section_projection(s) for s in manifest["execution"]["configSections"]]
        + [gen_config_resources(manifest)]
    )
    adapter = adapter[:start] + mid + "\n\n" + adapter[end:]
    binary = manifest.get("x-binary-name", name)
    out_dir = os.path.join(engine_dir, "cmd", f"{binary}-engine")
    os.makedirs(out_dir, exist_ok=True)
    with open(os.path.join(out_dir, "execution_run_generated.go"), "w", encoding="utf-8") as handle:
        handle.write(adapter)
    print(f"generated contract + adapter for {name} (binary: {binary}-engine)")


if __name__ == "__main__":
    main()
