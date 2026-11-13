"""Generate position-independent ARM entry stubs and relative descriptors from validated JSON."""
from sdk import choice


def literals(label, text):
    return label + ': .byte ' + ','.join(map(str, text.encode('ascii') + b'\0'))


def choice_pages(modules):
    arrows, code = [], []
    for index, doc in enumerate(modules):
        choice.validate(doc)
        row = doc['setting']
        opts = row['options']
        prefix = f'ox_choice_{index}_'
        def name(key):
            return prefix + key
        def ref(key):
            return f' .word {name(key)} - {name("desc")}'
        values = {'page':doc['title'], 'caption':row['label'],
                  'file':f"openx6-choice.{doc['id']}.{row['id']}.pref"}
        values['stage'] = values['file'] + '.next'
        arrows.extend([' mov x0,x19', ' adr x1,'+name('page'), ' adr x2,'+name('open'),
                       ' bl ext_add_arrow'])
        code.extend(['.balign 4', name('open')+':', ' bti c', ' adr x1,'+name('desc'),
                     ' b ox_choice_open', '.balign 4', name('desc')+':',
                     ref('caption'), ref('file'), ref('stage'),
                     ' .word '+str(len(opts)), ' .word '+str([o['id'] for o in opts].index(row['default']))])
        for i, opt in enumerate(opts):
            for key in ('id','label','note'):
                values[f'{key}{i}'] = opt[key]
                code.append(ref(f'{key}{i}'))
            code.append(ref(f'pick{i}'))
        for i in range(len(opts)):
            code.extend([name(f'pick{i}')+':',' bti c',' adr x1,'+name('desc'),
                         ' mov w2,#'+str(i),' b ox_choice_pick'])
        code.extend(literals(name(k), v) for k,v in values.items())
    return ('\n'.join(arrows)+'\n').encode(), ('\n'.join(code)+'\n').encode()
