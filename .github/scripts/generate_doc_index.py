import os
import sys
from pathlib import Path

def generate_version_list(versions, repo_slug):
    """Generates HTML list items for each version."""
    html = ""
    # Sort versions, putting 'latest' first, then others in reverse order
    sorted_versions = sorted([v for v in versions if v != 'latest'], reverse=True)
    if 'latest' in versions:
        sorted_versions.insert(0, 'latest')

    for version in sorted_versions:
        docs_link = f'<li><a href="./{version}/">{version}</a>'
        release_link = ""
        if version != 'latest':
            # Assumes tags are like 'v1.2.3'
            release_tag = f"v{version}"
            release_url = f"https://github.com/{repo_slug}/releases/tag/{release_tag}"
            release_link = f'<a href="{release_url}" class="release-link" target="_blank">View GitHub Release</a>'

        html += f'{docs_link}{release_link}</li>\n'
    return html

def main():
    template_path = Path(sys.argv[1])
    output_path = Path(sys.argv[2])
    versions_dir = Path(sys.argv[3])
    new_version = sys.argv[4]
    repo_slug = os.getenv("GITHUB_REPOSITORY")

    # Find existing versions by listing directories
    existing_versions = set()
    if versions_dir.exists():
        for item in versions_dir.iterdir():
            if item.is_dir() and not item.name.startswith('.'):
                existing_versions.add(item.name)

    # Add the new version being deployed
    existing_versions.add(new_version)

    # Read the template file
    template_content = template_path.read_text()

    # Generate the dynamic HTML list
    version_list_html = generate_version_list(existing_versions, repo_slug)

    # Replace the placeholder with the actual list
    final_html = template_content.replace("<!-- VERSION_LIST_PLACEHOLDER -->", version_list_html)

    # Write the new index.html file
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(final_html)
    print(f"Successfully generated index.html at {output_path}")

if __name__ == "__main__":
    main()