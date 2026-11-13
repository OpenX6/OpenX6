/* LCD preview only. The caller has validated the two-plane CHI buffers
 * and acquired DMA access. Samples use the same eye order as Fisheye. */
typedef struct { unsigned short x, y; } PreviewPoint;
extern const PreviewPoint preview_linear_grid[33][33] __attribute__((visibility("hidden")));
static void sample_linear(PIXEL *dst_y, PIXEL *dst_uv, const PIXEL *src_y,
                          const PIXEL *src_uv, unsigned eye_offset) {
    for (unsigned y=0; y<640; ++y) {
        unsigned row=y/20, fy=y%20;
        for (unsigned col=0; col<32; ++col) {
            const PreviewPoint *top=&preview_linear_grid[row][col];
            const PreviewPoint *bottom=&preview_linear_grid[row+1][col];
            int ax=top[0].x*(20-fy)+bottom[0].x*fy, ay=top[0].y*(20-fy)+bottom[0].y*fy;
            int bx=top[1].x*(20-fy)+bottom[1].x*fy, by=top[1].y*(20-fy)+bottom[1].y*fy;
            for (unsigned k=0; k<20; ++k) {
                unsigned x=col*20+k;
                unsigned sx=(ax*(20-k)+bx*k)/400, sy=(ay*(20-k)+by*k)/400;
                unsigned ix=sx>>4, iy=sy>>4, dx=sx&15, dy=sy&15;
                const PIXEL *p=src_y+iy*2048+eye_offset+ix;
                unsigned a=p[0]*(16-dx)+p[1]*dx, b=p[2048]*(16-dx)+p[2049]*dx;
                dst_y[640*y+x]=(a*(16-dy)+b*dy+128)>>8;
                if (!(x&1) && !(y&1)) {
                    const PIXEL *uv=src_uv+(iy/2)*2048+eye_offset+(ix&~1U);
                    dst_uv[(y/2)*640+x]=uv[0];
                    dst_uv[(y/2)*640+x+1]=uv[1];
                }
            }
        }
    }
    for (unsigned i=640*640;i<640*800;++i) dst_y[i]=BLACK;
    for (unsigned i=640*320;i<640*400;++i) dst_uv[i]=NEUTRAL;
}
