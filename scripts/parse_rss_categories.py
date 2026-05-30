#!/usr/bin/env python3
"""Parse dochub.sk.ru RSS feed and categorize documents with status."""
import sys
import xml.etree.ElementTree as ET
import json
from datetime import datetime

CATEGORIES = {
    'legislative_acts': 'Нормативные акты',
    'design_rules': 'Правила проектирования',
    'development': 'Развитие',
    'other': 'Прочее',
    'tenders': 'Закупки',
    'unactual_documents': 'Устаревшие',
    'anti_corruption': 'Антикоррупция',
    'cybersec_and_persdata': 'Кибербезопасность',
}

def main():
    data = sys.stdin.read()
    root = ET.fromstring(data)
    items = root.findall('.//item')

    docs = []
    for item in items:
        title = item.find('title').text or ''
        link = item.find('link').text or ''
        pubdate = item.find('pubDate').text or ''
        description = item.find('description').text or ''

        # Status determination
        title_upper = title.upper()
        is_outdated = 'УТРАТИЛ' in title_upper or 'УТРАТИЛО' in title_upper
        status = 'устарел' if is_outdated else 'действует'

        # Category from URL
        cat = 'documents'
        for slug, name in CATEGORIES.items():
            if slug in link:
                cat = name
                break

        # Extract doc ID from URL
        doc_id = link.split('/')[-1].replace('.aspx', '') if link else ''

        docs.append({
            'id': doc_id,
            'title': title,
            'link': link,
            'pubdate': pubdate,
            'status': status,
            'category': cat,
            'description': description[:200] if description else '',
        })

    # Print summary
    print(f"Всего документов: {len(docs)}")
    print(f"Действует: {sum(1 for d in docs if d['status'] == 'действует')}")
    print(f"Устарел: {sum(1 for d in docs if d['status'] == 'устарел')}")
    print()

    # Print by category
    for cat_name in CATEGORIES.values():
        cat_docs = [d for d in docs if d['category'] == cat_name]
        if cat_docs:
            print(f"\n{'='*60}")
            print(f"📁 {cat_name} ({len(cat_docs)} документов)")
            print(f"{'='*60}")
            for d in cat_docs:
                icon = '✅' if d['status'] == 'действует' else '❌'
                print(f"  {icon} {d['title'][:100]}")
                print(f"     {d['link']}")
                print(f"     📅 {d['pubdate']}")

    # Save as JSON for import
    with open('/tmp/skolkovo_docs.json', 'w') as f:
        json.dump(docs, f, ensure_ascii=False, indent=2)
    print(f"\n💾 Сохранено в /tmp/skolkovo_docs.json")

if __name__ == '__main__':
    main()
