"""
Get Binance Earn Flexible products APR
https://www.binance.com/ru/earn/apr-calculator
"""

from datetime import datetime
from pathlib import Path
import requests

URL_APY = "https://www.binance.com/bapi/earn/v2/friendly/finance-earn/calculator/product/list?asset={asset}&type=Flexible"
min_step = .00000001
current_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
output_file = Path("binance_apr.csv")

results = []
for asset in ("USDT", "BUSD", "DAI"):
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

if not output_file.exists():
    with open(output_file, 'w') as f:
        f.write("Time,Asset,APY,Bonus\n")
with open(output_file, 'a') as f:
    for i in results:
        f.write(f"{current_time},{i['asset']},{i['apy']},{i['bonus']}\n")
