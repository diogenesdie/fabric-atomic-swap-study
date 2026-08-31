#!/usr/bin/env python3
"""
Corrige os connection profiles YAML gerados pelo testbed do Weaver.

São dois defeitos independentes, ambos na seção `certificateAuthorities`.

PROBLEMA 1 — indentação do bloco literal do certificado.
O bloco literal do certificado é
aberto como item de lista (`- |`) e a primeira linha do PEM recebe 2 espaços de
indentação a mais que as seguintes. Em YAML, a indentação do bloco literal é
definida pela primeira linha; as linhas seguintes, menos indentadas, encerram o
bloco e passam a ser interpretadas como YAML — o que produz o erro
"could not find expected ':'".

    certificateAuthorities:
      ca.org1.network1.com:
        tlsCACerts:
          pem:
            - |
              -----BEGIN CERTIFICATE-----   <- 14 espaços (define o bloco)
            MIICKjCCAdGgAwIBAgIU...          <- 12 espaços: quebra o parse

Solução: normalizar todas as linhas do bloco para a indentação da primeira.

PROBLEMA 2 — ausência da entrada `registrar`.
Sem ela o SDK não consegue registrar novos usuários e falha com
"CA registrar not found". A identidade de bootstrap das CAs do testbed é
admin:adminpw (ver o comando `fabric-ca-server start -b admin:adminpw` em
docker/docker-compose-ca.yaml). Injetamos:

    registrar:
      enrollId: admin
      enrollSecret: adminpw

PROBLEMA 3 — `caName` divergente do nome real da CA.
O perfil declara `caName: ca-org1`, mas o servidor foi iniciado com
FABRIC_CA_SERVER_CA_NAME=ca.org1.<rede>.com (confirmável em
`curl -sk https://localhost:7054/cainfo`). O SDK então falha com
"Error Code: 19 - CA 'ca-org1' does not exist". O nome correto é exatamente a
chave da entrada no próprio perfil, então basta alinhar os dois.

Não usamos o connection-org1.json como alternativa porque ele não traz a seção
`orderers` — é um perfil incompleto.

Idempotente: rodar de novo num arquivo já corrigido não altera nada. Precisa ser
reexecutado depois de cada recriação das redes, pois os perfis são regerados.
"""

import sys
from pathlib import Path

try:
    import yaml
except ImportError:
    yaml = None  # a validação fica indisponível, a correção ainda funciona


def indent_of(line: str) -> int:
    return len(line) - len(line.lstrip(" "))


def fix_text(text: str) -> tuple[str, int]:
    """Devolve (texto corrigido, número de linhas reindentadas)."""
    lines = text.splitlines(keepends=True)
    out: list[str] = []
    fixes = 0
    i = 0

    while i < len(lines):
        line = lines[i]
        out.append(line)

        # Abertura de bloco literal como item de lista: "  - |"
        if line.rstrip("\n").rstrip().endswith("- |"):
            marker_indent = indent_of(line)
            i += 1
            if i >= len(lines):
                break

            # A primeira linha do bloco define a indentação correta
            first = lines[i]
            target = indent_of(first)
            # Precisa ser maior que a coluna do conteúdo do item de lista
            if target <= marker_indent + 1:
                target = marker_indent + 4
                if indent_of(first) != target:
                    out.append(" " * target + first.lstrip(" "))
                    fixes += 1
                else:
                    out.append(first)
            else:
                out.append(first)
            i += 1

            # Reindenta o restante do bloco até o fim do PEM
            while i < len(lines):
                cur = lines[i]
                stripped = cur.strip()
                if not stripped:
                    out.append(cur)
                    i += 1
                    continue
                # Uma chave YAML no nível do bloco encerra o certificado
                if indent_of(cur) <= marker_indent and ":" in stripped:
                    break
                if indent_of(cur) != target:
                    out.append(" " * target + cur.lstrip(" "))
                    fixes += 1
                else:
                    out.append(cur)
                i += 1
                if stripped == "-----END CERTIFICATE-----":
                    break
            continue

        i += 1

    return "".join(out), fixes


REGISTRAR_ID = "admin"
REGISTRAR_SECRET = "adminpw"


def add_registrars(text: str, missing: set[str]) -> tuple[str, int]:
    """Insere `registrar` nas CAs listadas em `missing`.

    Inserção textual (e não via yaml.dump) para preservar os blocos literais
    dos certificados exatamente como estão.
    """
    if not missing:
        return text, 0

    lines = text.splitlines(keepends=True)
    out: list[str] = []
    added = 0
    in_ca_section = False
    current_ca: str | None = None

    for line in lines:
        stripped = line.strip()
        ind = indent_of(line)

        # Entrada/saída da seção de topo certificateAuthorities
        if ind == 0 and stripped and not stripped.startswith("#"):
            in_ca_section = stripped.startswith("certificateAuthorities:")
            current_ca = None

        # Nome da CA (um nível dentro da seção)
        if in_ca_section and ind == 2 and stripped.endswith(":"):
            current_ca = stripped[:-1].strip()

        out.append(line)

        # Insere logo depois da linha `url:` da CA que precisa de registrar
        if (in_ca_section and current_ca in missing
                and stripped.startswith("url:")):
            pad = " " * ind
            out.append(f"{pad}registrar:\n")
            out.append(f"{pad}  enrollId: {REGISTRAR_ID}\n")
            out.append(f"{pad}  enrollSecret: {REGISTRAR_SECRET}\n")
            added += 1

    return "".join(out), added


def fix_ca_names(text: str, wrong: dict[str, str]) -> tuple[str, int]:
    """Reescreve `caName` das CAs em `wrong` ({nome_da_entrada: valor_atual})."""
    if not wrong:
        return text, 0

    lines = text.splitlines(keepends=True)
    out: list[str] = []
    changed = 0
    in_ca_section = False
    current_ca: str | None = None

    for line in lines:
        stripped = line.strip()
        ind = indent_of(line)

        if ind == 0 and stripped and not stripped.startswith("#"):
            in_ca_section = stripped.startswith("certificateAuthorities:")
            current_ca = None

        if in_ca_section and ind == 2 and stripped.endswith(":"):
            current_ca = stripped[:-1].strip()

        if (in_ca_section and current_ca in wrong
                and stripped.startswith("caName:")):
            out.append(" " * ind + f"caName: {current_ca}\n")
            changed += 1
            continue

        out.append(line)

    return "".join(out), changed


def cas_with_wrong_name(data: dict) -> dict[str, str]:
    """CAs cujo caName difere da chave da entrada."""
    cas = data.get("certificateAuthorities") or {}
    return {
        name: cfg.get("caName")
        for name, cfg in cas.items()
        if isinstance(cfg, dict) and cfg.get("caName") not in (None, name)
    }


def cas_without_registrar(data: dict) -> set[str]:
    cas = data.get("certificateAuthorities") or {}
    return {name for name, cfg in cas.items()
            if isinstance(cfg, dict) and not cfg.get("registrar")}


def process(path: Path) -> bool:
    """Corrige e valida um arquivo. Devolve True se ficou válido."""
    original = path.read_text(encoding="utf-8")
    text = original
    notes: list[str] = []

    # --- correção 1: indentação -----------------------------------------
    parses = False
    if yaml is not None:
        try:
            yaml.safe_load(text)
            parses = True
        except yaml.YAMLError:
            parses = False

    if not parses:
        text, fixes = fix_text(text)
        if yaml is not None:
            try:
                yaml.safe_load(text)
            except yaml.YAMLError as exc:
                print(f"  ✗ {path.name}: ainda inválido após reindentação: {exc}",
                      file=sys.stderr)
                return False
        notes.append(f"{fixes} linha(s) reindentada(s)")

    # --- correção 2: registrar ------------------------------------------
    if yaml is not None:
        data = yaml.safe_load(text)
        missing = cas_without_registrar(data)
        if missing:
            text, added = add_registrars(text, missing)
            try:
                data = yaml.safe_load(text)
            except yaml.YAMLError as exc:
                print(f"  ✗ {path.name}: inválido após inserir registrar: {exc}",
                      file=sys.stderr)
                return False
            still = cas_without_registrar(data)
            if still:
                print(f"  ✗ {path.name}: registrar não aplicado em {still}",
                      file=sys.stderr)
                return False
            notes.append(f"registrar adicionado em {added} CA(s)")

        # --- correção 3: caName --------------------------------------
        wrong = cas_with_wrong_name(data)
        if wrong:
            text, changed = fix_ca_names(text, wrong)
            try:
                data = yaml.safe_load(text)
            except yaml.YAMLError as exc:
                print(f"  ✗ {path.name}: inválido após corrigir caName: {exc}",
                      file=sys.stderr)
                return False
            still = cas_with_wrong_name(data)
            if still:
                print(f"  ✗ {path.name}: caName não corrigido em {still}",
                      file=sys.stderr)
                return False
            notes.append(f"caName corrigido em {changed} CA(s)")

        # Confere que a estrutura esperada sobreviveu
        for key in ("certificateAuthorities", "peers", "orderers"):
            if not data.get(key):
                print(f"  ✗ {path.name}: seção '{key}' ausente após correção",
                      file=sys.stderr)
                return False

    if text == original:
        print(f"  · {path.name}: já correto, nada a fazer")
        return True

    backup = path.with_suffix(path.suffix + ".orig")
    if not backup.exists():
        backup.write_text(original, encoding="utf-8")

    path.write_text(text, encoding="utf-8")
    print(f"  ✓ {path.name}: {'; '.join(notes)} (original em {backup.name})")
    return True


def main() -> int:
    if len(sys.argv) < 2:
        print("uso: fix_connection_profiles.py <arquivo.yaml> [...]",
              file=sys.stderr)
        return 2

    if yaml is None:
        print("  ! PyYAML não instalado: a correção será aplicada sem validação")

    ok = True
    for arg in sys.argv[1:]:
        path = Path(arg)
        if not path.is_file():
            print(f"  ✗ não encontrado: {arg}", file=sys.stderr)
            ok = False
            continue
        ok = process(path) and ok

    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
