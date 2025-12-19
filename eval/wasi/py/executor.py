import sys, json; d = json.load(sys.stdin); exec(d["code"], {"ctx": d["context"]})
