// Native Wi-Fi Settings subpage, using the same radio widget as Auto Power Off.
typedef unsigned char Obj;
typedef unsigned short Text;
typedef void (*Click)(Obj *);
typedef struct { Obj *owner; Click callback; } Slot;
extern void native_wifi_init(Obj *, unsigned long);
extern Obj *new_object(char *, void *);
extern void group_add(Obj *, Obj *, int);
extern Text *text_utf8(const char *, int);
extern void page_title(Obj *, Text *), arrow_title(Obj *, Text *), radio_title(Obj *, Text *);
extern void radio_selected(Obj *, int);
extern Slot new_slot(Obj *, Click);
extern void create_subpage(Obj *, char *, int, void *, void *, void *, void *);
#pragma GCC visibility push(hidden)
extern char page_class[], arrow_class[], radio_class[];
#pragma GCC visibility pop

#define FIELD(obj, offset, type) (*(type *)((obj) + (offset)))
#define TITLE "Auto Wi-Fi Off"
#define DIR "/data/x6-wifi-idle"
#define TMP "minutes.tmp"
#define NAME "minutes"
#define PREF DIR "/" NAME
#define LABEL(s) text_utf8(s, sizeof(s)-1)

static long call(long n, long a, long b, long c, long d) {
    register long x8 __asm__("x8")=n, x0 __asm__("x0")=a, x1 __asm__("x1")=b;
    register long x2 __asm__("x2")=c, x3 __asm__("x3")=d;
    __asm__ volatile("svc 0":"+r"(x0):"r"(x8),"r"(x1),"r"(x2),"r"(x3):"memory","cc");
    return x0;
}
static const unsigned char durations[4]={0,2,5,10};
static int value(const char *path) {
    long fd=call(56,-100,(long)path,0x88800,0); // NOFOLLOW|CLOEXEC|NONBLOCK
    if(fd<0) return -1;
    unsigned char bytes[2];
    long count=call(63,fd,(long)bytes,2,0);
    call(57,fd,0,0,0);
    if(count==1) for(int i=0;i<4;i++) if(bytes[0]==durations[i]) return i;
    return -1;
}
static int selected(void) {
    int index=value(PREF);
    if(index<0) index=value("/run/x6-wifi-idle/inherited");
    return index<0 ? 0 : index;
}
static void persist(int index) {
    long made=call(34,-100,(long)DIR,0700,0); // mkdirat, EEXIST is expected
    if(made<0 && made!=-17) return;
    long dir=call(56,-100,(long)DIR,0x8c000,0); // DIRECTORY|NOFOLLOW|CLOEXEC
    if(dir<0) return;
    long fd=call(56,dir,(long)TMP,0x88241,0600); // O_TRUNC|O_CREAT|O_WRONLY
    if(fd>=0) {
        int ok=call(64,fd,(long)&durations[index],1,0)==1 && call(82,fd,0,0,0)==0;
        if(call(57,fd,0,0,0)!=0) ok=0;
        if(ok && call(38,dir,(long)TMP,dir,(long)NAME)==0) call(82,dir,0,0,0);
        else call(35,dir,(long)TMP,0,0);
    }
    call(57,dir,0,0,0);
}
static void choose(Obj *page,int index);
static void never(Obj *p){choose(p,0);}
static void two(Obj *p){choose(p,1);}
static void five(Obj *p){choose(p,2);}
static void ten(Obj *p){choose(p,3);}
static Click handler(int i) {
    // Avoid a relocated table of absolute pointers in a position-independent patch.
    if(i==0)return never;
    if(i==1)return two;
    if(i==2)return five;
    return ten;
}
static void choose(Obj *page,int index) {
    persist(index);
    int current=selected();
    for(Obj *r=FIELD(page,0x60,Obj*);r;r=FIELD(r,0x20,Obj*)) {
        if(FIELD(r,0,char*)!=radio_class) continue;
        Slot s=FIELD(r,0x4c0,Slot);
        if(s.owner==page) for(int i=0;i<4;i++) if(s.callback==handler(i)) radio_selected(r,i==current);
    }
}
static void open_choices(Obj *parent) {
    if(FIELD(parent,0xa8,Obj*)) return;
    create_subpage(parent,page_class,0,0,0,0,0);
    Obj *page=FIELD(parent,0xa8,Obj*);
    if(!page || FIELD(page,0,char*)!=page_class) return;
    page_title(page+0x338,LABEL(TITLE));
    int index=selected();
    for(int i=0;i<4;i++) {
        Obj *r=new_object(radio_class,0);
        if(!r)return;
        group_add(page,r,0);
        Text *label;
        if(i==0)label=LABEL("Never");
        else if(i==1)label=LABEL("2 min");
        else if(i==2)label=LABEL("5 min");
        else label=LABEL("10 min");
        radio_title(r,label);
        radio_selected(r,i==index);
        FIELD(r,0x4c0,Slot)=new_slot(page,handler(i));
    }
}
void wifi_settings_init(Obj *page,unsigned long argument) {
    native_wifi_init(page,argument);
    Obj *row=new_object(arrow_class,0);
    if(!row)return;
    group_add(page,row,0);
    arrow_title(row,LABEL(TITLE));
    FIELD(row,0x4c0,Slot)=new_slot(page,open_choices);
}
