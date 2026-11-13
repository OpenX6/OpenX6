"""Create the one private module preference directory before the relabel helper."""
import os
import stat

flags = os.O_DIRECTORY | os.O_NOFOLLOW | os.O_RDONLY | os.O_CLOEXEC
data = os.open('/data', flags)
parent = os.open('x6-owner-features', flags, dir_fd=data)
for fd in (data, parent):
    st = os.fstat(fd)
    if st.st_uid != 0 or st.st_mode & 0o022:
        raise ValueError('unsafe preference ancestor')
try:
    os.mkdir('extensions-v1', 0o700, dir_fd=parent)
except FileExistsError:
    pass
child = os.open('extensions-v1', flags, dir_fd=parent)
st = os.fstat(child)
if st.st_uid != 0 or stat.S_IMODE(st.st_mode) != 0o700:
    raise ValueError('unsafe preference directory')
os.fsync(parent)
