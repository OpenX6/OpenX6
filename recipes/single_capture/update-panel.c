// X6 Embedded Wizard SD update panel. The global root is square, but the
// visible panel is 480x600 (600x480 after native display rotation).
typedef unsigned char Obj;
typedef unsigned short Text;
typedef struct {int x1,y1,x2,y2;} Rect;
typedef struct {
    unsigned last_poll,phase_since;
    int phase,code,detail,seconds,width,height;
    Obj *app,*background,*title,*translation,*value,*hint,*track,*fill;
} Panel;
#pragma GCC visibility push(hidden)
extern Panel panel;
extern char global_obj[],global_class[],rect_class[],text_class[],font_resource[],font_class[],body_resource[],number_resource[];
#pragma GCC visibility pop
extern long timers(void);
extern unsigned ticks(void);
extern int buffer_direction(void), direction_vertical(int);
extern Obj *get_auto(void *,void *), *new_object(void *,void *), *load_resource(void *,void *);
extern void lock_obj(Obj *), unlock_obj(Obj *), group_add(Obj *,Obj *,int),group_remove(Obj *,Obj *);
extern Rect bounds(Obj *);
extern void set_bounds(Obj *,Rect),rect_color(Obj *,unsigned),rect_visible(Obj *,int);
extern void text_font(Obj *,Obj *),text_color(Obj *,unsigned),alignment(Obj *,int);
extern Text *new_utf8(const char *,int);
extern void text_string(Obj *,Text *);

static long sys(long nr,long a,long b,long c,long d) {
    register long x0 __asm__("x0")=a,x1 __asm__("x1")=b,x2 __asm__("x2")=c,x3 __asm__("x3")=d,x8 __asm__("x8")=nr;
    __asm__ volatile("svc 0":"+r"(x0):"r"(x1),"r"(x2),"r"(x3),"r"(x8):"memory","cc");return x0;
}
static int read_status(unsigned char *detail) {
    int fd=sys(56,-100,(long)"/run/x6-sd-update-notice",0x88800,0);
    if(fd<0)return 0;
    unsigned char b[2]={0,0};
    if(sys(32,fd,5,0,0)==-11 && sys(63,fd,(long)b,2,0)>0) {
        if(!((b[0]>=1&&b[0]<=7)||(b[0]>=16&&b[0]<=116)||(b[0]>=128&&b[0]<=228)))b[0]=0;
    } else b[0]=0;
    sys(57,fd,0,0,0);*detail=b[1]<=101?b[1]:0;return b[0];
}
static void label(Obj *v,const char *s){
    int n=0;while(s[n])n++;text_string(v,new_utf8(s,n));
}
static void drop(Obj **slot){
    Obj *v=*slot;if(!v)return;
    if(*(Obj**)(v+0x30)==panel.app)group_remove(panel.app,v);
    *slot=0;unlock_obj(v);
}
static void hide(void){
    if(!panel.app)return;
    drop(&panel.fill);drop(&panel.track);drop(&panel.hint);drop(&panel.value);
    drop(&panel.translation);drop(&panel.title);drop(&panel.background);
    unlock_obj(panel.app);panel.app=0;panel.phase=panel.code=0;panel.width=0;
}
static Obj *view(char *cls){Obj *v=new_object(cls,0);if(v)lock_obj(v);return v;}
static void style(Obj *v,Obj *font,unsigned color){text_font(v,font);text_color(v,color);alignment(v,18);}
static int attach(Obj *app){
    panel.app=app;lock_obj(app);
    panel.background=view(rect_class);panel.title=view(text_class);panel.translation=view(text_class);
    panel.value=view(text_class);panel.hint=view(text_class);panel.track=view(rect_class);panel.fill=view(rect_class);
    if(!panel.background||!panel.title||!panel.translation||!panel.value||!panel.hint||!panel.track||!panel.fill){hide();return 0;}
    Obj *body=load_resource(body_resource,font_class);
    if(!body){hide();return 0;}style(panel.title,body,0xffffffff);style(panel.translation,body,0xffffffff);
    Obj *number=load_resource(number_resource,font_class);
    if(!number){hide();return 0;}style(panel.value,number,0xfff2dc67);
    Obj *small=load_resource(font_resource,font_class);
    if(!small){hide();return 0;}style(panel.hint,small,0xffe4e3df);
    label(panel.hint,"Keep SD in  /  请勿拔卡");
    rect_color(panel.background,0xff211c19);rect_color(panel.track,0xff554e4a);rect_color(panel.fill,0xfff2dc67);
    group_add(app,panel.background,0);group_add(app,panel.title,0);group_add(app,panel.translation,0);
    group_add(app,panel.value,0);group_add(app,panel.hint,0);group_add(app,panel.track,0);group_add(app,panel.fill,0);
    return 1;
}
static char *digits(char *dst,unsigned n){
    if(n>=10)dst=digits(dst,n/10);
    *dst++='0'+n%10;return dst;
}
static void update(int phase,int pct,unsigned secs,int w,int h){
    const char *en,*zh;
    if(phase==128){en="Copying update";zh="正在复制更新包";}
    else if(phase==16){en="Preparing SD card";zh="正在准备 SD 卡空间";}
    else if(phase==1){en="Checking update";zh="正在检查更新包";}
    else if(phase==2){en="Preparing storage";zh="正在准备暂存空间";}
    else if(phase==3){en="Verifying copy";zh="正在校验更新副本";}
    else if(phase==5){en="Reading source";zh="正在读取源文件";}
    else if(phase==6){en="Final check";zh="正在执行最终校验";}
    else if(phase==7){en="Confirming input";zh="正在确认更新包";}
    else {en="Starting updater";zh="正在启动升级程序";}
    if(panel.phase!=phase||!panel.width){label(panel.title,en);label(panel.translation,zh);}
    char number[20],*p=number;
    // Embedded Wizard consumes the first percent sign as an escape.
    if(pct>=0){p=digits(p,pct);*p++='%';*p++='%';}
    else {p=digits(p,secs/60);*p++=':';*p++='0'+(secs%60)/10;*p++='0'+secs%10;}
    *p=0;label(panel.value,number);
    int margin=w/32, left=margin,right=w-margin;
    int height=h*3/4,top=(h-height)/2;
    int pad=(right-left)/20,l=left+pad,r=right-pad;
    set_bounds(panel.background,(Rect){left,top,right,top+height});
    // Nonoverlapping single-line views, including room for CJK ascenders.
    set_bounds(panel.title,(Rect){l,top+height*5/100,r,top+height*21/100});
    set_bounds(panel.translation,(Rect){l,top+height*22/100,r,top+height*38/100});
    set_bounds(panel.value,(Rect){l,top+height*39/100,r,top+height*64/100});
    int bar=top+height*69/100;
    set_bounds(panel.hint,(Rect){l,top+height*76/100,r,top+height*94/100});
    set_bounds(panel.track,(Rect){l,bar,r,bar+9});
    if(pct>0)set_bounds(panel.fill,(Rect){l,bar,l+(r-l)*pct/100,bar+9});
    rect_visible(panel.track,pct>=0);rect_visible(panel.fill,pct>0);
}
long entry(void){
    long result=timers();unsigned now=ticks();
    if((unsigned)(now-panel.last_poll)<350)return result;
    panel.last_poll=now;unsigned char extra=0;int code=read_status(&extra);
    if(!code){hide();return result;}
    Obj *global=get_auto(global_obj,global_class);Obj *app=global?*(Obj**)(global+0x7d0):0;
    if(app!=panel.app)hide();if(!app)return result;
    Rect root=bounds(app);int rw=root.x2-root.x1,rh=root.y2-root.y1;
    int d=buffer_direction(),vertical=direction_vertical(d);
    int w=vertical?480:600,h=vertical?600:480;
    if(rw<w)w=rw;if(rh<h)h=rh;
    if(w<280||h<300){hide();return result;}
    if(!panel.app&&!attach(app))return result;
    int phase=code>=128?128:(code>=16?16:code);
    int pct=phase>=16?code-phase:(extra?extra-1:-1);
    if(panel.phase!=phase)panel.phase_since=now;
    int secs=(now-panel.phase_since)/1000;
    if(panel.code!=code||panel.detail!=extra||w!=panel.width||h!=panel.height||(pct<0&&secs!=panel.seconds)){
        update(phase,pct,secs,w,h);panel.code=code;panel.detail=extra;panel.phase=phase;
        panel.width=w;panel.height=h;panel.seconds=secs;
    }
    return result;
}
