/* Factory EW picker adapter. Its model and bitmap holders belong to one open page. */
typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef unsigned long up;
typedef unsigned char Obj;
typedef struct { Obj *owner; int (*get)(Obj *); void (*set)(Obj *,int); } Ref;
extern Obj *new_object(void *,void *), *auto_object(void *,void *);
extern u16 *new_utf8(const char *,int), *option_text(Obj *,int);
static u16 *new_string(const char *s){int bytes=0;while(s[bytes])bytes++;return new_utf8(s,bytes);}
extern void resource_set(Obj *,Obj *,u16 *), notify(Ref *,int), refresh_desc(Obj *), set_stock(Obj *,int), set_title(Obj *,u16 *), set_body(Obj *,u16 *), set_frame(Obj *,int), set_bitmap(Obj *,Obj *), lock_obj(Obj *),unlock_obj(Obj *);
extern u8 imgbase[] __attribute__((visibility("hidden")));
#define PTR(p,o) (*(Obj **)((p)+(o)))
#define INT(p,o) (*(int *)((p)+(o)))
enum {FF=1,PANO=2,FISH=100,NATIVE=101,LINEAR=4};
static int scope(Obj *sp) {
    if(!sp)return 0;
    int lens=INT(sp,0x15f0),mode=INT(sp,0x1558);
    if(lens==3 && mode==2)return FF;
    if(lens==1 && mode==0)return PANO;
    return 0;
}
static Obj *spec(void){return auto_object(PTR(imgbase,0x7166558),PTR(imgbase,0x7160fc0));}
static long sc(long n,long a,long b,long c,long d) {
    register long nr __asm__("x8")=n;register long x __asm__("x0")=a;register long y __asm__("x1")=b;
    register long z __asm__("x2")=c;register long w __asm__("x3")=d;
    __asm__ volatile("svc 0":"+r"(x):"r"(nr),"r"(y),"r"(z),"r"(w):"memory","cc");return x;
}
#define ROOT "/run/x6-nativefish/"
static int exists(const char *path){return sc(48,-100,(long)path,0,0)==0;}
static void flag(const char *path,int yes){
    if(yes){long fd=sc(56,-100,(long)path,0xc1,0600);if(fd>=0)sc(57,fd,0,0,0);}
    else sc(35,-100,(long)path,0,0);
}
static void clear_pano(void){flag(ROOT"pano-choice",0);}
static int pano_choice(void){
    char c=0;long n=sc(78,-100,(long)ROOT"pano-choice",(long)&c,1);
    return n==1 && c=='F'?FISH:n==1 && c=='L'?LINEAR:NATIVE;
}
static int write_choice(int x) {
    if(x==NATIVE){clear_pano();return pano_choice()==NATIVE;}
    if(x!=FISH && x!=LINEAR)return 0;
    flag(ROOT"next-choice",0);
    const char *s=x==LINEAR?"L":"F";
    if(sc(36,(long)s,-100,(long)ROOT"next-choice",0)<0)return 0;
    if(sc(38,-100,(long)ROOT"next-choice",-100,(long)ROOT"pano-choice")<0){flag(ROOT"next-choice",0);return 0;}
    return 1;
}
#define CHOICE_DIR "/data/x6-owner-features/fov-choice-v1"
static const char *saved_path(int mode) {
    return mode == FF ? CHOICE_DIR "/freeframe" : CHOICE_DIR "/panorama";
}
static int saved_view(int mode) {
    char buf[2];
    if (sc(78,-100,(long)saved_path(mode),(long)buf,sizeof(buf)) == 1) {
        if (buf[0]=='F') return FISH;
        if (mode==PANO && buf[0]=='L') return LINEAR;
    }
    return NATIVE;
}
static void sync_directory(const char *path) {
    long fd = sc(56,-100,(long)path,0x8c000,0);
    if (fd >= 0) {
        sc(82,fd,0,0,0);
        sc(57,fd,0,0,0);
    }
}
static void remember_view(int mode, int value) {
    const char *path = saved_path(mode);
    if (value == FISH || (mode == PANO && value == LINEAR)) {
        if (saved_view(mode)==value) return;
        long rc = sc(34,-100,(long)CHOICE_DIR,0700,0);
        if (rc == 0) sync_directory("/data/x6-owner-features");
        else if (rc != -17) return;
        const char *tmp = CHOICE_DIR "/next";
        sc(35,-100,(long)tmp,0,0);
        if (sc(36,(long)(value==LINEAR?"L":"F"),-100,(long)tmp,0) < 0) return;
        if (sc(38,-100,(long)tmp,-100,(long)path) < 0) {
            sc(35,-100,(long)tmp,0,0);
            return;
        }
    } else if (sc(35,-100,(long)path,0,0) < 0) {
        return;
    }
    sync_directory(CHOICE_DIR);
}
static void mirror_eye(int eye){if(eye==0||eye==1)flag(ROOT"pano-front",eye==0);}
#define MIRROR_DIR "/data/x6-owner-features/flat-mirror-v1"
#define MIRROR_PREF MIRROR_DIR "/enabled"
static void update_mirror_gate(Obj *sp) {
    int active = sp && INT(sp,0x15f0)==3 &&
                 (INT(sp,0x1558)==0 || INT(sp,0x1558)==2) && exists(MIRROR_PREF);
    if (active) sc(34,-100,(long)"/run/x6-nativefish",0700,0);
    if (exists(ROOT "unmirror-video") != active) flag(ROOT "unmirror-video",active);
}
void update_role(int eye){mirror_eye(eye);update_mirror_gate(spec());}

typedef struct { Obj *owner; void (*method)(Obj *); } Slot;
extern void group_add(Obj *,Obj *,int), set_checked(Obj *,int);
extern void switch_title(Obj *,u16 *);
extern Slot new_slot(Obj *,void (*)(Obj *));
static void mirror_clicked(Obj *row) {
    int on = row[0xb88] != 0;
    if (on && sc(34,-100,(long)MIRROR_DIR,0700,0)==0)
        sync_directory("/data/x6-owner-features");
    if (exists(MIRROR_PREF)!=on) {
        flag(MIRROR_PREF,on);
        sync_directory(MIRROR_DIR);
    }
    set_checked(row,exists(MIRROR_PREF));
    update_mirror_gate(spec());
}
void append_mirror_row(Obj *page) {
    Obj *row = new_object(imgbase+0x715cf38,0);
    if (!row) return;
    group_add(page,row,0);
    switch_title(row,new_string("Disable Flat Mode Mirror"));
    set_checked(row,exists(MIRROR_PREF));
    *(Slot *)(row+0x4c0)=new_slot(row,mirror_clicked);
}
int allow_scope(Obj *sp){
    update_mirror_gate(sp);
    int k=scope(sp);
    if(k)sc(34,-100,(long)"/run/x6-nativefish",0700,0);
    if(k!=FF && exists(ROOT"enabled"))flag(ROOT"enabled",0);
    if(k!=PANO && pano_choice()!=NATIVE)clear_pano();
    if(k==FF) {
        int want=saved_view(k)==FISH;
        if(want!=exists(ROOT"enabled"))flag(ROOT"enabled",want);
    }
    if(k==PANO) {
        int wanted=saved_view(k);
        if(pano_choice()!=wanted)write_choice(wanted);
    }
    return k;
}
static Obj *stock_model(void){
    Obj *parent=auto_object(PTR(imgbase,0x7160f28),PTR(imgbase,0x71699e0));return parent?parent+0x2938:0;
}
static int chinese(void){
    Obj *m=stock_model();if(!m||!PTR(m,0x68))return 0;
    u16 *s=(u16 *)PTR(PTR(m,0x68),0x58);
    return s && (*s>=0x4e00 && *s<=0x9fff);
}
static const char *caption(int v){return v==FISH?"Fisheye":v==NATIVE?"Native":"Linear";}
static const char *description(int v,int cn){
    if(cn){
        if(v==FISH)return "直接预览当前镜头的鱼眼画面，跳过取景拼接和防抖。可见范围随拍摄档位变化。";
        if(v==LINEAR)return "将当前镜头的画面展平，让直线保持平直。取景范围较窄。";
        return "使用相机原有的全景取景方式，可在屏幕上调整观看方向。";
    }
    if(v==FISH)return "Fisheye preview, showing what the sensor captured natively. Framing depends on the recording mode.";
    if(v==LINEAR)return "Keeps straight lines straight in a narrower view of the current lens.";
    return "Use the camera's original panoramic preview. Swipe the screen to look around.";
}
static int selected(Obj *group){
    (void)group;Obj *sp=spec();int k=scope(sp);
    if(k==PANO)return pano_choice();
    if(k==FF && exists(ROOT"enabled"))return FISH;
    return sp?INT(sp,0x159c):3;
}
static void choose(Obj *group,int v){
    Obj *sp=spec();int k=scope(sp);if(!k)return;
    if(k==PANO){
        Obj *stock= *(Obj **)(imgbase+0x7341510);
        for(int i=0;stock && i<2048;i++,stock=PTR(stock,0)){
            Obj *m=PTR(stock,16);if(m && PTR(m,0)==imgbase+0x70fa858){mirror_eye(INT(m,0x30));break;}
        }
        if(!write_choice(v))return;
    }else if(v==FISH){flag(ROOT"enabled",1);}
    else if(v>=0 && v<=17){flag(ROOT"enabled",0);set_stock(sp,v);}
    else return;
    remember_view(k,v);
    Ref r={group,selected,choose};notify(&r,0);
    Ref live={sp, (int (*)(Obj *))PTR(imgbase,0x7164418),
                  (void (*)(Obj *,int))PTR(imgbase,0x7169678)};
    notify(&live,0);
    refresh_desc(group+0x10c8);
}
static Obj *new_holder(Obj *m,int field,Obj *cls){
    Obj *h=new_object(cls,0);if(h)PTR(m,field)=h;return h;
}
extern const u8 glyphs[2][60][8] __attribute__((visibility("hidden")));
extern Obj *make_bitmap(int format,up size,int frames,int surfaces);
extern Obj *lock_bitmap(Obj *,int,up,up,int,int);
extern void unlock_bitmap(Obj *),free_bitmap(Obj *);
extern const u8 shortcut_labels[2][48][6] __attribute__((visibility("hidden")));
extern Obj *load_resource(Obj *,Obj *);
static int draw_glyph(Obj *res,const u8 *pixels,int edge) {
    up size=(up)edge|((up)edge<<32);
    Obj *b=make_bitmap(0,size,0,1);
    if(!b)return 0;
    Obj *locked=lock_bitmap(b,0,0,size,0,1);
    if(!locked){free_bitmap(b);return 0;}
    int stride=INT(locked,12);
    if(!PTR(locked,0)||stride<edge*4||stride>8192){
        unlock_bitmap(locked);free_bitmap(b);return 0;
    }
    for(int y=0;y<edge;y++){
        u32 *line=(u32 *)(PTR(locked,0)+y*stride);
        const u8 *row=pixels+y*((edge+7)/8);
        for(int x=0;x<edge;x++)line[x]=(row[x/8]&(1u<<(x%8)))?0xffffffffu:0u;
    }
    unlock_bitmap(locked);resource_set(res,b,0);return 1;
}
static void icon(Obj *icons,int i){
    Obj *res=new_holder(icons,0x28+i*8,imgbase+0x7032df0);
    if(res)draw_glyph(res,&glyphs[i][0][0],60);
}
static int idx_of(int v){
    const u8 vals[12]={11,1,0,2,12,3,4,13,14,15,16,17};
    for(int i=0;i<12;i++)if(vals[i]==v)return i;
    return -1;
}
/* The compact button and the popup use different native bitmap sizes. */
void shortcut(Obj *button,int original_index){
    if(scope(spec())){
        int choice=selected(0);
        if(choice==FISH || choice==NATIVE){
            Obj *res=new_object(imgbase+0x7032df0,0);
            if(res){
                lock_obj(res);
                int drawn=draw_glyph(res,&shortcut_labels[choice==NATIVE][0][0],48);
                if(drawn){set_frame(button,0);set_bitmap(button,res);}
                unlock_obj(res);
                if(drawn)return;
            }
        }else{
            int index=idx_of(choice);
            if(index>=0)original_index=index;
        }
    }
    Obj *stock=load_resource(PTR(imgbase,0x7166330),PTR(imgbase,0x7164bc8));
    set_bitmap(button,stock);
    set_frame(button,original_index);
}
Obj *make_model(Obj *stock,Obj *group){
    if(!stock||stock!=stock_model())return stock;
    int k=scope(PTR(stock,0x40));if(!k)return stock;
    Obj *org_i=PTR(stock,0x60),*org_s=PTR(stock,0x68),*mask=PTR(stock,0x78);
    if(!org_i||!org_s||!mask||INT(org_i,0x20)!=12||INT(org_s,0x20)!=12)return stock;
    Obj *m=new_object(PTR(stock,0),0);if(!m)return stock;
    lock_obj(m);
    Obj *ints=new_holder(m,0x60,PTR(org_i,0)); Obj *names=new_holder(m,0x68,PTR(org_s,0));
    Obj *icons=new_holder(m,0x70,imgbase+0x7029828);
    if(!ints||!names||!icons){unlock_obj(m);return stock;}
    int a[15],n=0;
    if(k==PANO){a[n++]=NATIVE;a[n++]=LINEAR;}
    else {
      for(int i=0;i<12;i++)if(mask[0x28+i])a[n++]=INT(org_i,0x28+4*i);
      if(!n){a[n++]=3;a[n++]=4;}
    }
    a[n++]=FISH;
    INT(ints,0x20)=INT(names,0x20)=n;INT(icons,0x20)=2;
    for(int j=0;j<n;j++){
        int v=a[j],orig=idx_of(v);INT(ints,0x28+4*j)=v;
        PTR(names,0x28+8*j)=orig<0?(Obj*)new_string(caption(v)):PTR(org_s,0x28+8*orig);
    }
    icon(icons,0);icon(icons,1);
    *(Ref *)(m+0x40)=(Ref){group,selected,choose};
    unlock_obj(m);return m;
}
int draw_row(Obj *list,Obj *button,int v){
    Obj *m=PTR(list,0x178);
    if(!m||((Ref*)(m+0x40))->set!=choose)return 0;
    Obj *b;int i;
    if(v==FISH||v==NATIVE){b=PTR(PTR(m,0x70),0x28+(v==NATIVE?8:0));i=0;}
    else{b=PTR(list,0xdf8);i=idx_of(v);if(i<0)return 0;}
    set_frame(button,i);set_bitmap(button,b);return 1;
}
int describe(Obj *desc){
    Obj *sp=spec();int k=scope(sp);if(!k)return 0;
    int v=selected(0);
    if(k==FF && v!=FISH){
        /* On added formats the stock visible mask is empty. Its filtered index
           then resolves Linear to Dewarp. Use the original localized strings
           at the index of the selected value in the unfiltered OEM table. */
        Obj *m=stock_model();
        if(!m || (v!=3 && v!=LINEAR))return 0;
        Obj *mask=PTR(m,0x78),*names=PTR(m,0x68),*descriptions=PTR(m,0x80);
        if(!mask||!names||!descriptions)return 0;
        int index=idx_of(v);
        if(mask[0x28+index])return 0;
        u16 *title=option_text(names,index),*body=option_text(descriptions,index);
        if(!title||!body)return 0;
        set_title(desc+0xca8,title);
        set_body(desc+0x6c0,body);
        return 1;
    }
    set_title(desc+0xca8,new_string(caption(v)));
    set_body(desc+0x6c0,new_string(description(v,chinese())));
    return 1;
}
