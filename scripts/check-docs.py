#!/usr/bin/env python3
"""Check local Markdown links and compare documented reports with the demo.

External links are not fetched. The demo only renders synthetic data; it does
not load credentials, scan infrastructure, or send telemetry.
"""
import difflib
import html
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import unquote, urlsplit


def prose_lines(text):
    """Yield numbered Markdown lines outside fenced code blocks."""
    fence = ""
    for number, line in enumerate(text.splitlines(), 1):
        match = re.match(r"^ {0,3}(`{3,}|~{3,})(.*)$", line)
        if match:
            marker, rest = match.groups()
            if not fence:
                fence = marker
            elif marker[0] == fence[0] and len(marker) >= len(fence) and not rest.strip():
                fence = ""
            continue
        if not fence:
            yield number, line


def heading_anchors(text):
    anchors = set()
    for _, line in prose_lines(text):
        match = re.match(r"^ {0,3}#{1,6}\s+(.+?)\s*#*\s*$", line)
        if not match:
            continue
        heading = re.sub(r"<[^>]+>", "", match.group(1))
        slug = re.sub(r"[^\w -]", "", html.unescape(heading).lower()).replace(" ", "-")
        anchor, suffix = slug, 1
        while anchor in anchors:
            anchor = f"{slug}-{suffix}"
            suffix += 1
        anchors.add(anchor)
    return anchors


def document_paths(root):
    result = subprocess.run(
        ["git", "ls-files", "-co", "--exclude-standard", "-z"],
        cwd=root, capture_output=True, text=True, check=True,
    )
    paths = {root / name for name in result.stdout.split("\0") if Path(name).suffix in (".md", ".mdc")}
    # Internal historical documents are ignored by Git but can exist in a clone.
    paths.update(root.glob("enhancement/**/*.md"))
    return sorted(path for path in paths if path.is_file())


def check_links(root, paths):
    errors = []
    anchors = {}
    inline = re.compile(r"!?\[[^\]\n]*\]\(\s*(<[^>\n]+>|[^\s)]+)(?:\s+[\"'][^\n]*?[\"'])?\s*\)")
    reference = re.compile(r"^ {0,3}\[[^\]]+\]:\s*(<[^>\n]+>|\S+)")
    for path in paths:
        for number, line in prose_lines(path.read_text(encoding="utf-8")):
            links = list(inline.finditer(line))
            definition = reference.match(line)
            if definition:
                links.append(definition)
            for match in links:
                target = html.unescape(match.group(1).strip("<>"))
                url = urlsplit(target)
                if url.scheme or url.netloc:
                    continue
                destination = path.parent / unquote(url.path) if url.path else path
                problem = ""
                if not destination.exists():
                    problem = "missing local target"
                elif url.fragment and destination.suffix in (".md", ".mdc"):
                    if destination not in anchors:
                        anchors[destination] = heading_anchors(destination.read_text(encoding="utf-8"))
                    if unquote(url.fragment) not in anchors[destination]:
                        problem = "missing heading anchor"
                if problem:
                    errors.append(f"{path.relative_to(root)}:{number}: {problem}: {target}")
    return errors


def check_examples(root):
    errors = []
    for flags, fixture in (([], "examples/report.txt"), (["--output=json"], "examples/report.json")):
        result = subprocess.run(
            ["go", "run", "./scripts/demo", *flags], cwd=root,
            capture_output=True, text=True, encoding="utf-8", check=True, timeout=300,
        )
        expected = (root / fixture).read_text(encoding="utf-8")
        if expected != result.stdout:
            difference = "".join(difflib.unified_diff(
                expected.splitlines(keepends=True), result.stdout.splitlines(keepends=True),
                fromfile=fixture, tofile="production demo",
            ))
            errors.append(f"{fixture}: does not match the production demo\n{difference}")
        if not flags:
            readme = (root / "README.md").read_text(encoding="utf-8")
            section = re.search(r"(?ms)^## See a sample[ \t]*\n(.*?)(?=^## |\Z)", readme)
            sample = re.search(r"(?ms)^```text\n(.*?)^```[ \t]*$", section.group(1)) if section else None
            if sample is None or sample.group(1) != result.stdout:
                errors.append("README.md: the text block under 'See a sample' must match the production demo")
    return errors


def main():
    root = Path(__file__).resolve().parent.parent
    try:
        paths = document_paths(root)
        errors = check_links(root, paths) + check_examples(root)
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"Documentation check could not complete: {error}", file=sys.stderr)
        if isinstance(error, subprocess.CalledProcessError) and error.stderr:
            print(error.stderr, file=sys.stderr)
        return 1
    if errors:
        print("Documentation check failed:\n" + "\n".join(errors), file=sys.stderr)
        return 1
    print(f"Documentation checks passed ({len(paths)} Markdown documents, both report examples, and the README sample).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
