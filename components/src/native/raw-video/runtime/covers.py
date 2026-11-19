"""Small per-take cover, sampled from Bayer and used only by the on-camera browser."""
import array
import ctypes as c
import os
import struct
from pathlib import Path

WIDTH, HEIGHT = 176, 120


def sample_cover(width, height, row, cfa, black, white):
    if sorted(cfa) != [0, 1, 1, 2] or min(width, height) < 2 or white <= black:
        raise ValueError('Unsupported CFA')
    scale = min(WIDTH/width, HEIGHT/height)
    w, h = max(1, round(width*scale)), max(1, round(height*scale))
    result = bytearray(WIDTH*HEIGHT*3)
    lut = bytes(min(255, round(255*(max(0, i-black)/(white-black))**0.45)) for i in range(65536))
    for y in range(h):
        sy = min(height-2, int((y+.5)*height/h)//2*2)
        a,b = row(sy), row(sy+1)
        for x in range(w):
            sx = min(width-2, int((x+.5)*width/w)//2*2)
            values = [a[sx], a[sx+1], b[sx], b[sx+1]]
            rgb = [0,0,0]
            for site,ch in enumerate(cfa): rgb[ch] += values[site]
            rgb[1] //= 2
            off = 3*((y+(HEIGHT-h)//2)*WIDTH+x+(WIDTH-w)//2)
            result[off:off+3] = bytes(lut[v] for v in rgb)
    return result


def frame_cover(buffer, calibration):
    view=memoryview(buffer).cast('B').cast('H')
    return sample_cover(3840,3840,lambda y:view[y*3840:(y+1)*3840],
                        calibration['cfa'],calibration['black'],calibration['white'])


def photo_cover(path):
    # The camera's RAW Only photos contain 16-bit strips in a classic TIFF.
    lib=c.CDLL('libtiff.so.6')
    lib.TIFFOpen.argtypes=[c.c_char_p,c.c_char_p];lib.TIFFOpen.restype=c.c_void_p
    lib.TIFFReadEncodedStrip.argtypes=[c.c_void_p,c.c_uint32,c.c_void_p,c.c_ssize_t]
    lib.TIFFReadEncodedStrip.restype=c.c_ssize_t
    lib.TIFFClose.argtypes=[c.c_void_p]
    with open(path,'rb') as f:
        head=f.read(8)
        if head[:4]!=b'II*\0':raise ValueError('unsupported TIFF')
        f.seek(struct.unpack_from('<I',head,4)[0]);n=struct.unpack('<H',f.read(2))[0]
        if n>1024:raise ValueError('IFD too large')
        fields={}
        for _ in range(n):
            tag,kind,count,value=struct.unpack('<HHI4s',f.read(12))
            if tag not in (256,257,258,262,277,278,33422,50714,50717):continue
            fmt={1:'B',3:'H',4:'I'}.get(kind)
            if not fmt or count>16:continue
            size=count*struct.calcsize(fmt);pos=f.tell()
            if size>4:f.seek(struct.unpack('<I',value)[0]);value=f.read(size);f.seek(pos)
            fields[tag]=struct.unpack('<'+fmt*count,value[:size])
    w,h,depth,space,channels,rows=(fields[t][0] for t in (256,257,258,262,277,278))
    if depth!=16 or space!=32803 or channels!=1 or not (2<=w<=32000 and 2<=h<=32000 and 0<rows*w*2<=32<<20):
        raise ValueError('unsupported RAW layout')
    handle=lib.TIFFOpen(os.fsencode(path),b'r')
    if not handle:raise ValueError('TIFF open failed')
    try:
        buf=(c.c_uint16*(rows*w))();cached=-1
        raw=memoryview(buf).cast('B').cast('H')
        def row(y):
            nonlocal cached
            idx=y//rows
            if idx!=cached:
                required=min(rows,h-idx*rows)*w*2
                if lib.TIFFReadEncodedStrip(handle,idx,buf,required)!=required:raise ValueError('short strip')
                cached=idx
            offset=(y%rows)*w
            return raw[offset:offset+w]
        return sample_cover(w,h,row,fields[33422],fields.get(50714,(64,))[0],fields.get(50717,(16383,))[0])
    finally:lib.TIFFClose(handle)
