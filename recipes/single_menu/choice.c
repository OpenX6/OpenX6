// Native extension radio pages for the verified 1.2.36 GUI ABI.
typedef unsigned long usize;
typedef unsigned int u32;
typedef void *object;
typedef void (*callback)(object);
struct slot { object owner; callback invoke; };
struct option { int id, label, note, pick; };
struct descriptor { int caption, file, stage, count, initial; struct option options[]; };

#pragma GCC visibility push(hidden)
extern char radio_class[], dualtext_class[];
extern object ext_create_page(object, const char *);
extern object new_object(void *, int);
extern void group_add(object, object, int);
extern object new_string(const char *);
extern void radio_title(object, object), radio_selected(object, int);
extern void dualtext_title(object, object), dualtext_desc(object, object);
extern struct slot new_slot(object, callback);

static const char *at(const struct descriptor *d, int relative) {
    return (const char *)d + relative;
}
static usize length(const char *text) {
    usize n = 0;
    while (text[n]) ++n;
    return n;
}
static long sys(long nr, long a, long b, long c, long d, long e) {
    register long x0 asm("x0") = a, x1 asm("x1") = b, x2 asm("x2") = c,
                  x3 asm("x3") = d, x4 asm("x4") = e, x8 asm("x8") = nr;
    asm volatile("svc 0" : "+r"(x0) : "r"(x1),"r"(x2),"r"(x3),"r"(x4),"r"(x8) : "memory");
    return x0;
}
enum { FLOCK=32, MKDIRAT=34, UNLINKAT=35, OPENAT=56, CLOSE=57, READ=63, WRITE=64,
       FSTAT=80, FSYNC=82, RENAMEAT=38, NOFOLLOW=0x20000, NONBLOCK=0x800,
       CLOEXEC=0x80000, DIRECTORY=0x4000, EXCLUSIVE_NEW=0xc1 };
static int safe_fd(long fd, int directory) {
    // linux/aarch64 struct stat: mode/nlink/uid at 16/20/24.
    u32 st[32] __attribute__((aligned(8)));
    if (sys(FSTAT,fd,(long)st,0,0,0) || st[6] != 0) return 0;
    return directory ? (st[4] & 0170022) == 0040000 :
                       (st[4] & 0177777) == 0100600 && st[5] == 1;
}
// The bootstrap classifier expects feature namespaces to contain directories.
// Create our child lazily and never populate or reinterpret the root namespace.
static int open_directory(int create) {
    const int flags=DIRECTORY|NOFOLLOW|CLOEXEC;
    long parent=sys(OPENAT,-100,(long)"/data/x6-owner-features",flags,0,0);
    if (parent<0) return -1;
    long dir=-1;
    if (safe_fd(parent,1)) {
        long child=(long)"extensions-v1";
        dir=sys(OPENAT,parent,child,flags,0,0);
        if (dir==-2 && create) {
            long made=sys(MKDIRAT,parent,child,0700,0,0);
            dir=(made==0 || made==-17) ? sys(OPENAT,parent,child,flags,0,0) : -1;
        }
        if (dir>=0 && (!safe_fd(dir,1) || (create && sys(FSYNC,parent,0,0,0,0)))) {
            sys(CLOSE,dir,0,0,0,0);
            dir=-1;
        }
    }
    sys(CLOSE,parent,0,0,0,0);
    return dir;
}
static int current_at(int dir, const struct descriptor *d) {
    char buf[49];
    long fd = sys(OPENAT,dir,(long)at(d,d->file),NOFOLLOW|CLOEXEC|NONBLOCK,0,0);
    if (fd < 0) return fd == -2 ? d->initial : -1;
    long n = safe_fd(fd,0) ? sys(READ,fd,(long)buf,sizeof(buf),0,0) : -1;
    sys(CLOSE,fd,0,0,0,0);
    for (int i=0; i<d->count; ++i) {
        const char *id = at(d,d->options[i].id);
        if ((long)length(id) != n) continue;
        long j=0;
        while (j<n && id[j]==buf[j]) ++j;
        if (j==n) return i;
    }
    // An upgrade may retire a valid option. Keep it on disk until the owner picks again.
    if (n<1 || n>48 || buf[0]<'a' || buf[0]>'z') return -1;
    for (long i=1;i<n;++i)
        if (!((buf[i]>='a' && buf[i]<='z') || (buf[i]>='0' && buf[i]<='9') || buf[i]=='_'))
            return -1;
    return -2;
}
static int current(const struct descriptor *d) {
    int dir=open_directory(0);
    if (dir==-2) return d->initial;
    if (dir<0) return -1;
    int selection=current_at(dir,d);
    sys(CLOSE,dir,0,0,0,0);
    return selection;
}
static int save(const struct descriptor *d,int pick) {
    int dir=open_directory(1), success=0;
    if (dir<0) return 0;
    const char *stage=at(d,d->stage), *file=at(d,d->file);
    const char *id=at(d,d->options[pick].id);
    long size=length(id);
    // Directory flock serializes cleanup/publication across GUI instances. Closing releases it.
    if (sys(FLOCK,dir,6,0,0,0) || current_at(dir,d)==-1) goto done;
    long rc=sys(UNLINKAT,dir,(long)stage,0,0,0);
    if (rc && rc!=-2) goto done;
    long fd=sys(OPENAT,dir,(long)stage,EXCLUSIVE_NEW|NOFOLLOW|CLOEXEC,0600,0);
    if (fd<0) goto done;
    int written=sys(WRITE,fd,(long)id,size,0,0)==size && sys(FSYNC,fd,0,0,0,0)==0;
    if (sys(CLOSE,fd,0,0,0,0) || !written ||
        sys(RENAMEAT,dir,(long)stage,dir,(long)file,0)) {
        sys(UNLINKAT,dir,(long)stage,0,0,0);
        goto done;
    }
    success=sys(FSYNC,dir,0,0,0,0)==0;
done:
    sys(CLOSE,dir,0,0,0,0);
    return success;
}
static object child(object page,void *cls) {
    object widget=new_object(cls,0);
    if (widget) group_add(page,widget,0);
    return widget;
}
static const char *note(const struct descriptor *d,int index) {
    if (index == -2) return "Choose a new option";
    return index<0 ? "Preference unavailable" : at(d,d->options[index].note);
}
void ox_choice_open(object parent,const struct descriptor *d) {
    object page=ext_create_page(parent,at(d,d->caption));
    if (!page) return;
    int index=current(d);
    for (int i=0;i<d->count;++i) {
        object row=child(page,radio_class);
        if (!row) return;
        const struct option *o=&d->options[i];
        radio_title(row,new_string(at(d,o->label)));
        radio_selected(row,index==i);
        *(struct slot *)((char *)row+0x4c0)=new_slot(page,(callback)at(d,o->pick));
    }
    object info=child(page,dualtext_class);
    if (info) {
        dualtext_title(info,new_string("Note"));
        dualtext_desc(info,new_string(note(d,index)));
    }
}
void ox_choice_pick(object page,const struct descriptor *d,int pick) {
    if ((unsigned)pick >= (unsigned)d->count) return;
    int saved=save(d,pick);
    int index=current(d);
    object row=*(object *)((char *)page+0x60);
    for (int i=0;row && i<128;++i,row=*(object *)((char *)row+0x20)) {
        if (*(object *)row==radio_class) {
            struct slot *s=(struct slot *)((char *)row+0x4c0);
            for (int j=0;j<d->count;++j)
                if (s->owner==page && s->invoke==(callback)at(d,d->options[j].pick))
                    radio_selected(row,j==index);
        } else if (*(object *)row==dualtext_class) {
            dualtext_desc(row,new_string(saved ? note(d,index) : "Save failed"));
        }
    }
}
#pragma GCC visibility pop
