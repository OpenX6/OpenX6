// Live shutter and ISO on the native shooting page.
typedef unsigned char Object;
typedef struct { int x,y,r,b; } Rect;
typedef struct { Object *owner; void (*click)(Object *); } Slot;
typedef struct { unsigned long stamp; int shutter,iso; unsigned auto_mask; } Reading;
typedef struct { Object *page,*bar; unsigned tick,heartbeat,mask; int value[2],forced; unsigned long since; } State;
_Static_assert(sizeof(State)<=160,"state allocation");
#pragma GCC visibility push(hidden)
extern State state;
extern char switch_class[],text_class[],font_resource[],font_class[],focus_id[],focus_class[],
    live_class[],capture_class[],app_id[],app_class[],bar_class[];
#pragma GCC visibility pop
extern void *get_auto(void*,void*),*new_object(void*,void*),*load_resource(void*,void*);
extern void group_add(Object*,Object*,int),group_remove(Object*,Object*),lock_obj(Object*),unlock_obj(Object*);
extern void text_string(Object*,void*),text_font(Object*,void*),text_color(Object*,unsigned),alignment(Object*,int);
extern Rect bounds(Object*);
extern void set_bounds(Object*,Rect),switch_title(Object*,void*),switch_checked(Object*,int);
extern void *new_string(const char*);
extern Slot new_slot(Object*,void (*)(Object*));
extern unsigned ticks(void);
extern long previous(void);
#define PREF "/data/x6-owner-features/show-exposure"
static long sys(long nr,long a,long b,long c,long d) {
    register long x0 __asm__("x0")=a,x1 __asm__("x1")=b,x2 __asm__("x2")=c,x3 __asm__("x3")=d,x8 __asm__("x8")=nr;
    __asm__ volatile("svc 0":"+r"(x0):"r"(x1),"r"(x2),"r"(x3),"r"(x8):"memory","cc");
    return x0;
}
static int enabled(void) {
    long fd=sys(56,-100,(long)PREF,0xa0800,0);
    if(fd<0)return 0;
    sys(57,fd,0,0,0);return 1;
}
static void clicked(Object *row) {
    if(row[0xb88]){
        long fd=sys(56,-100,(long)PREF,0xa08c1,0600);
        if(fd>=0){sys(82,fd,0,0,0);sys(57,fd,0,0,0);}
    }else sys(35,-100,(long)PREF,0,0);
    long dir=sys(56,-100,(long)"/data/x6-owner-features",0xa4000,0);
    if(dir>=0){sys(82,dir,0,0,0);sys(57,dir,0,0,0);}
    switch_checked(row,enabled());
}
void exposure_row(Object *page) {
    Object *row=new_object(switch_class,0);
    if(!row)return;
    group_add(page,row,0);
    switch_title(row,new_string("Show Shutter & ISO"));
    switch_checked(row,enabled());
    *(Slot*)(row+0x4c0)=new_slot(row,clicked);
}
static unsigned long monotonic_ms(void) {
    long ts[2];
    if(sys(113,1,(long)ts,0,0)<0)return 0;
    return ts[0]*1000UL+ts[1]/1000000;
}
static Object *shooting_page(void) {
    Object *focus=get_auto(focus_id,focus_class);
    Object *p=focus?*(Object**)(focus+0x28):0;
    if(!p){
        Object *app=get_auto(app_id,app_class);
        p=app?*(Object**)(app+0x7e8):0;
    }
    if(!p||!*(Object**)(p+0x30)||*(Object**)(p+0xa8))return 0;
    void *type=*(void**)p;
    return type==live_class||type==capture_class?p:0;
}
extern void item(Object*,int,int,void*),stock_shutter(Object*,void*),stock_iso(Object*,void*);
extern void visible(Object*,int);
extern int is_visible(Object*);
static Object *native_bar(Object *page){
    for(Object *v=page?*(Object**)(page+0x68):0;v;v=*(Object**)(v+0x28))
        if(*(void**)v==bar_class)return v;
    return 0;
}
static void restore(unsigned mask){
    if(mask&1)stock_shutter(state.bar,0);
    if(mask&2)stock_iso(state.bar,0);
    state.mask &= ~mask;
}
static void detach(void){
    if(state.page){
        restore(state.mask);
        if(state.forced)visible(state.bar,0);
        unlock_obj(state.page);
    }
    state.page=state.bar=0;state.forced=0;
}
static char *number(char *p,unsigned long n){
    char reversed[22]; unsigned i=0;
    do{reversed[i++]='0'+n%10;n/=10;}while(n);
    while(i)*p++=reversed[--i];
    return p;
}
static void decimal(char *p,unsigned n){
    p=number(p,n/1000);
    if(n%1000){
        *p++='.';*p++='0'+n/100%10;
        if(n%100){*p++='0'+n/10%10;if(n%10)*p++='0'+n%10;}
    }
    *p=0;
}
static void show(int index,int value){
    unsigned flag=1u<<index;
    if((state.mask&flag)&&state.value[index]==value&&state.bar[0x258+2+index])return;
    char text[32],*p=text;
    if(!value){*p++='-';*p++='-';*p=0;}
    else if(index)*number(p,value)=0;
    else if(value<0){*p++='1';*p++='/';decimal(p,-(long)value);}
    else decimal(p,value);
    item(state.bar,2+index,1,new_string(text));
    state.value[index]=value;state.mask |=flag;
}
#define DEMAND "/run/openx6-exposure-visible"
static void heartbeat(unsigned long now){
    long fd=sys(56,-100,(long)DEMAND,0xa0041,0644);
    if(fd>=0){sys(64,fd,(long)&now,8,0);sys(57,fd,0,0,0);}
}
long exposure_tick(void){
    long ret=previous();
    unsigned t=ticks();
    if(t-state.tick<250)return ret;
    state.tick=t;
    Object *page=enabled()?shooting_page():0;
    if(state.page!=page)detach();
    if(!page)return ret;
    unsigned long now=monotonic_ms();
    if(!state.bar){
        Object *bar=native_bar(page);
        if(!bar)return ret;
        lock_obj(page);state.page=page;state.bar=bar;state.since=now;
        heartbeat(now);state.heartbeat=t;
    }else if(t-state.heartbeat>=750){
        heartbeat(now);state.heartbeat=t;
    }
    Reading reading={0};
    unsigned char raw[21];
    long fd=sys(56,-100,(long)"/run/openx6-exposure",0xa0800,0);
    if(fd>=0){
        long n=sys(63,fd,(long)raw,sizeof raw,0);sys(57,fd,0,0,0);
        if(n==20)__builtin_memcpy(&reading,raw,20);
    }
    unsigned mask=reading.auto_mask&3;
    if(reading.stamp<state.since||reading.stamp>now||now-reading.stamp>1400){
        mask=state.mask;reading.shutter=reading.iso=0;
    }
    restore(state.mask&~mask);
    if(mask&1)show(0,reading.shutter);
    if(mask&2)show(1,reading.iso);
    // Firmware normally hides the whole bar while rolling. Only take over its
    // visibility on a mounted shooting page while automatic readings are shown.
    if(mask&&!is_visible(state.bar)){visible(state.bar,1);state.forced=1;}
    if(!mask&&state.forced){visible(state.bar,0);state.forced=0;}
    return ret;
}
