"""Derive the native recording profile for FreeFrame and panoramic normal video."""
import json
from .common import require, sha

PROFILE_PATH = '/usr/share/x6-ff-pano-sole-main-v1.json'
ORIGINAL_PATH = '/ins/etc/ins_mediad/ins_media_stream_prd.json'

def build(original):
    require(sha(original) == 'e75c97ac5f949d3b706b2ff1604e352e18dad39752da2b39bb366a0664e13163',
            'Unexpected input recording profile')
    document = json.loads(original)
    streams = document['stream_profile']
    selected = []
    for group in streams['video_stream_groups']:
        name = group['group_id']
        if not (name.startswith('All_NormalVideo_') or '_FreeFrameVideo_' in name):
            continue
        ports = {port['type']: port for port in group['stream_param']}
        if 'main_stream' not in ports or 'sub_stream' not in ports:
            continue
        port = ports['sub_stream']
        use = port.get('stream_use_case')
        require('sub_yuv_stream' not in ports and port['format'] == 'NV12'
                and port['usage'] == 'video' and use in ('VideoPanoSBS','VideoInstaFilter','VideoInstaFrame'),
                'Unexpected auxiliary port for ' + name)
        port.update(type='sub_yuv_stream', yuv_stream_usage='video')
        if use != 'VideoPanoSBS':
            port['fps'] = [1000,1001]
        port.setdefault('stream_dispatch_list', [])
        selected.append(group)
    require(len(selected) == len({group['group_id'] for group in selected}) == 89,
            'Incomplete recording profile')
    by_name = {g['group_id']: g for g in selected}
    for lens in ('Front', 'Back'):
        g = by_name[lens + '_FreeFrameVideo_16K_4x3_60']
        donor = by_name[lens + '_FreeFrameVideo_6K_4x3_30']
        slot, = [i for i,s in enumerate(g['stream_param']) if s['type']=='sub_yuv_stream']
        g['stream_param'][slot], = [s for s in donor['stream_param'] if s['type']=='sub_yuv_stream']
    streams['video_stream_groups'] = selected
    streams['photo_stream_groups'] = []
    return json.dumps(document, separators=(',', ':')).encode()
