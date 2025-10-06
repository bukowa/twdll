import os
import sys
import re
from pathlib import Path
from packaging.version import parse as parse_version

def get_changelog_for_version(version, changelog_text):
    """Parses CHANGELOG.md and extracts changes for a specific version."""
    pattern = re.compile(f"## \\[{version}\\](.*?)(?=\\n## \\[|$)", re.DOTALL)
    match = pattern.search(changelog_text)
    if not match: return "<p>No changelog entry found.</p>"
    content = match.group(1).strip()
    html_content = "<ul>\n"
    for line in content.splitlines():
        line = line.strip()
        if line.startswith(('-', '*')): html_content += f"  <li>{line[1:].strip()}</li>\n"
        elif line.startswith(('###', '##')): html_content += f"</ul><h4>{line.lstrip('#').strip()}</h4><ul>\n"
    html_content += "</ul>"
    return html_content

def generate_version_list(versions, repo_slug, changelog_text, repo_path):
    """Generates HTML list items with new sorting and naming."""
    html = ""

    # Sortowanie pozostaje bez zmian
    tagged_versions = [v for v in versions if v != 'nightly']
    tagged_versions.sort(key=lambda v: parse_version(v), reverse=True)
    sorted_list = tagged_versions
    if 'nightly' in versions:
        sorted_list.append('nightly')

    for version in sorted_list:
        is_nightly = (version == 'nightly')
        display_name = "Dev Build" if is_nightly else version
        css_class = ' class="dev-build"' if is_nightly else ''

        # ZMIANA: Całkowicie nowa struktura HTML dla każdego elementu listy, pasująca do nowego szablonu
        html += f"<li{css_class}>\n"
        html += '    <div class="version-header">\n'

        # ZMIANA: Wersja jest teraz tylko tekstem w <span>, a nie linkiem
        html += f'        <span class="version-label">{display_name}</span>\n'

        # ZMIANA: Kontener na dwa nowe przyciski
        html += '        <div class="version-actions">\n'

        # ZMIANA: Nowy przycisk "View Documentation"
        doc_url = f"./{version}/"
        html += f'            <a href="{doc_url}" class="button doc-link">View Documentation</a>\n'

        # ZMIANA: Zaktualizowany przycisk pobierania z nowymi klasami CSS
        if is_nightly:
            release_url = f"https://github.com/{repo_slug}/releases/tag/nightly"
            download_text = "Download Dev Build"
        else:
            release_tag = f"v{version}"
            release_url = f"https://github.com/{repo_slug}/releases/tag/{release_tag}"
            download_text = "Download Release"

        html += f'            <a href="{release_url}" class="button release-link" target="_blank">{download_text}</a>\n'

        html += '        </div>\n' # Zamknięcie version-actions
        html += '    </div>\n' # Zamknięcie version-header

        # Ta część pozostaje bez zmian
        if not is_nightly:
            html += '    <details class="changelog-details">\n'
            html += '        <summary>View Changes</summary>\n'
            html += f"        {get_changelog_for_version(version, changelog_text)}\n"
            html += '    </details>\n'

        html += "</li>\n"

    return html

def main():
    if len(sys.argv) < 6:
        print("Usage: python generate_index.py <template_path> <output_path> <versions_dir> <new_version> <repo_path>")
        sys.exit(1)

    template_path = Path(sys.argv[1])
    output_path = Path(sys.argv[2])
    versions_dir = Path(sys.argv[3])
    new_version = sys.argv[4]
    repo_path = Path(sys.argv[5])

    repo_slug = os.getenv("GITHUB_REPOSITORY")
    if not repo_slug:
        sys.exit("Error: GITHUB_REPOSITORY environment variable not set.")

    changelog_path = repo_path / "CHANGELOG.md"
    changelog_text = changelog_path.read_text() if changelog_path.exists() else ""

    existing_versions = {item.name for item in versions_dir.iterdir() if item.is_dir() and not item.name.startswith('.')}
    if new_version:
        existing_versions.add(new_version)

    template_content = template_path.read_text()
    version_list_html = generate_version_list(existing_versions, repo_slug, changelog_text, repo_path)
    final_html = template_content.replace("<!-- VERSION_LIST_PLACEHOLDER -->", version_list_html)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(final_html)

    print(f"Successfully generated index.html at {output_path}")

if __name__ == "__main__":
    main()