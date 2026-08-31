#!/usr/bin/env python3
"""
Lê o estado de um ativo nos ledgers e resolve a titularidade para nome de
usuário.

Motivo de existir: no simpleasset o campo `owner` de um bond é o **certificado
X.509 do dono em base64**, não o nome do usuário. Para classificar o desfecho de
um experimento (quem ficou com o quê) é preciso decodificar o certificado e
extrair o enrollment ID, que o Fabric CA embute no atributo
`hf.EnrollmentID` da extensão de atributos.

Uso:
    ledger_state.py owner-of <json-do-ativo>
    ledger_state.py enrollment-id <certificado-base64>

Também serve como biblioteca: `from ledger_state import enrollment_id`.
"""

import base64
import json
import re
import sys

# O Fabric CA grava os atributos como JSON dentro de uma extensão X.509.
# Em vez de depender de uma biblioteca de ASN.1, extraímos o campo por regex do
# PEM decodificado — o JSON aparece em claro no corpo do certificado.
_ENROLLMENT_RE = re.compile(rb'"hf\.EnrollmentID"\s*:\s*"([^"]+)"')


def enrollment_id(cert_b64: str) -> str | None:
    """Extrai o enrollment ID (nome do usuário) de um certificado em base64."""
    if not cert_b64:
        return None
    try:
        pem = base64.b64decode(cert_b64)
    except Exception:
        return None

    # O PEM traz o DER em base64; decodificamos para achar os atributos.
    body = b"".join(
        line for line in pem.splitlines()
        if b"-----" not in line
    )
    try:
        der = base64.b64decode(body)
    except Exception:
        der = b""

    for blob in (der, pem):
        match = _ENROLLMENT_RE.search(blob)
        if match:
            return match.group(1).decode("utf-8", "replace")

    # Alternativa: o CN do sujeito costuma ser o próprio nome do usuário
    cn = re.search(rb"\x06\x03U\x04\x03[\x0c\x13].(.{1,64}?)[\x30\x31]", der)
    if cn:
        return cn.group(1).decode("utf-8", "replace")

    return None


def owner_of(asset_json: str) -> str | None:
    """Recebe o JSON de um ativo e devolve o nome do dono."""
    try:
        data = json.loads(asset_json)
    except json.JSONDecodeError:
        return None
    owner = data.get("owner") or data.get("Owner") or ""
    # Tokens guardam o dono em claro; bonds guardam o certificado
    if owner and len(owner) < 64 and "-----" not in owner:
        return owner
    return enrollment_id(owner)


def main() -> int:
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2

    cmd, arg = sys.argv[1], sys.argv[2]

    if cmd == "owner-of":
        result = owner_of(arg)
    elif cmd == "enrollment-id":
        result = enrollment_id(arg)
    else:
        print(f"comando desconhecido: {cmd}", file=sys.stderr)
        return 2

    if result is None:
        print("DESCONHECIDO")
        return 1
    print(result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
