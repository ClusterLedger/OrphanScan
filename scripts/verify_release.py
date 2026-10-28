#!/usr/bin/env python3
"""Validate staged archives, smoke-test the native binary, and test installation.

Installer downloads are served from staged files through a temporary curl shim;
this never contacts GitHub, cloud APIs, or a telemetry service.
"""
import hashlib
import os
from pathlib import Path, PurePosixPath
import platform
import subprocess
import sys
import tarfile
import tempfile
import zipfile


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    root = Path(sys.argv[1]).resolve()
    repository = Path(__file__).resolve().parent.parent
    manifest = {}
    for line in (root / "checksums.txt").read_text().splitlines():
        digest, name = line.split(maxsplit=1)
        manifest[name.strip().lstrip("*")] = digest
    required = {"README.md", "LICENSE", "SECURITY.md", "docs/installation.md",
                "docs/usage.md", "docs/detection-and-pricing.md", "docs/report-format.md",
                "docs/telemetry.md", "docs/permissions/aws-iam-policy.json", "examples/report.json"}
    for system in ("Linux", "Darwin", "Windows"):
        for architecture in ("x86_64", "arm64"):
            extension = "zip" if system == "Windows" else "tar.gz"
            name = f"orphanscan_{system}_{architecture}.{extension}"
            archive = root / name
            assert name in manifest and sha256(archive) == manifest[name], f"checksum failed: {name}"
            if extension == "zip":
                with zipfile.ZipFile(archive) as contents:
                    names = set(contents.namelist())
            else:
                with tarfile.open(archive) as contents:
                    names = set(contents.getnames())
            assert all(not PurePosixPath(item).is_absolute() and ".." not in PurePosixPath(item).parts for item in names), name
            binary = "orphanscan.exe" if system == "Windows" else "orphanscan"
            assert required | {binary} <= names, (name, "missing archive contents", (required | {binary}) - names)

    system = platform.system()
    machine = platform.machine().lower()
    architecture = "arm64" if machine in ("arm64", "aarch64") else "x86_64"
    extension = "zip" if system == "Windows" else "tar.gz"
    native_archive = root / f"orphanscan_{system}_{architecture}.{extension}"
    filename = "orphanscan.exe" if system == "Windows" else "orphanscan"
    with tempfile.TemporaryDirectory(prefix="orphanscan-release-") as temporary:
        directory = Path(temporary)
        binary = directory / filename
        if extension == "zip":
            with zipfile.ZipFile(native_archive) as contents:
                binary.write_bytes(contents.read(filename))
        else:
            with tarfile.open(native_archive) as contents:
                stream = contents.extractfile(filename)
                assert stream is not None
                with stream:
                    binary.write_bytes(stream.read())
        binary.chmod(0o755)
        subprocess.run([sys.executable, str(repository / "scripts/smoke.py"), str(binary)], check=True)
        if system in ("Linux", "Darwin"):
            shim = directory / "bin"
            shim.mkdir()
            curl = shim / "curl"
            curl.write_text('''#!/usr/bin/env python3
import os
from pathlib import Path
import shutil
import sys
arguments = sys.argv[1:]
url = arguments[-1]
if "-o" not in arguments:
    raise SystemExit("staged installer check requires a pinned version")
out = arguments[arguments.index("-o") + 1]
shutil.copyfile(Path(os.environ["ORPHANSCAN_STAGED_RELEASE"]) / url.rsplit("/", 1)[-1], out)
''')
            curl.chmod(0o755)
            install_dir = directory / "installed with spaces"
            env = os.environ.copy()
            env.update(PATH=str(shim) + os.pathsep + env["PATH"],
                       VERSION="v0.1.0-snapshot", INSTALL_DIR=str(install_dir),
                       ORPHANSCAN_STAGED_RELEASE=str(root))
            subprocess.run(["sh", str(repository / "install.sh")], env=env, check=True, stdout=subprocess.DEVNULL)
            assert sha256(install_dir / "orphanscan") == sha256(binary), "installed binary changed"
    print("Verified all six archive checksums and documentation, native binary smoke tests, and staged Unix installation.")


if __name__ == "__main__":
    main()
