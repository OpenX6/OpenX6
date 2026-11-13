from .common import require,sha
PATH='/etc/nginx/conf.d/fileops.conf'
def build(stock,route):
    require(sha(stock)=='ffd3dddf3b6c5f3cabe16f69315503cfdf3008a963a6cd5e5595548899d83361','Unexpected firmware upload routing')
    a=stock.index(b'    location /upload_fw {')
    b=stock.index(b'    location @upload_complete {',a)
    return stock[:a]+route+stock[b:]
