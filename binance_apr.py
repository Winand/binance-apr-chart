"""
Get Binance Earn Flexible products APR
https://www.binance.com/ru/earn/apr-calculator
"""

import json
import sqlite3
from datetime import datetime
from pathlib import Path

import requests

URL_APY = "https://www.binance.com/bapi/earn/v2/friendly/finance-earn/calculator/product/list?asset={asset}&type=Flexible"
min_step = .00000001
now = datetime.now()  # now(tz=timezone.utc) https://blog.ganssle.io/articles/2019/11/utcnow.html
current_timestamp = int(now.timestamp())
output_file = Path("binance_apr.sqlite")


with open("assets.json") as f:
    asset_list = json.load(f)

results = []
for asset in asset_list:
    resp = requests.get(URL_APY.format(asset=asset))
    assert resp.ok
    d = resp.json()
    assert d['success']
    f = d['data']['savingFlexibleProduct'][0]
    apy = float(f['apy'])
    marketApr = float(f['marketApr'])
    result = {
        'asset': f['asset'],
        'apy': apy,
        # 'marketApr': float(f['marketApr']),
        'bonus': round(apy - marketApr, 3) if abs(apy - marketApr) >= min_step else 0,
    }
    results.append(result)

results.sort(key=lambda i: i['apy'], reverse=True)
print(results)

sql = f"insert into apr (time, asset, apy, bonus) values ({current_timestamp}, :asset, :apy, :bonus)"
conn = sqlite3.connect(output_file)
with conn:  # commit() afterwards
    conn.execute("""
        create table if not exists 'apr' (
			time datetime, asset string, apy float, bonus float,
			primary key (time, asset)
		) without rowid
    """)
    conn.executemany(sql, results)
conn.close()
