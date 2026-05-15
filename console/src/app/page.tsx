"use client";

import Link from "next/link";

export default function Home() {
  return (
    <div className="p-8">
      {/* 页面标题 */}
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-foreground">仪表盘</h1>
        <p className="text-muted-foreground mt-2">Headscale 管理控制台概览</p>
      </div>

      {/* 统计卡片 */}
      <div className="grid grid-cols-4 gap-6 mb-8">
        <Link href="/nodes" className="group">
          <div className="bg-card rounded-2xl p-6 border border-border hover:border-primary/50 transition-all hover:shadow-lg hover:shadow-primary/5">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-blue-500 to-cyan-500 flex items-center justify-center">
                <svg className="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
                </svg>
              </div>
              <span className="text-3xl font-bold text-foreground">0</span>
            </div>
            <h3 className="font-medium text-foreground">节点总数</h3>
            <p className="text-sm text-muted-foreground">已连接设备</p>
          </div>
        </Link>

        <Link href="/users" className="group">
          <div className="bg-card rounded-2xl p-6 border border-border hover:border-primary/50 transition-all hover:shadow-lg hover:shadow-primary/5">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-violet-500 to-purple-500 flex items-center justify-center">
                <svg className="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
                </svg>
              </div>
              <span className="text-3xl font-bold text-foreground">0</span>
            </div>
            <h3 className="font-medium text-foreground">用户总数</h3>
            <p className="text-sm text-muted-foreground">注册用户</p>
          </div>
        </Link>

        <Link href="/api-keys" className="group">
          <div className="bg-card rounded-2xl p-6 border border-border hover:border-primary/50 transition-all hover:shadow-lg hover:shadow-primary/5">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-amber-500 to-orange-500 flex items-center justify-center">
                <svg className="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                </svg>
              </div>
              <span className="text-3xl font-bold text-foreground">0</span>
            </div>
            <h3 className="font-medium text-foreground">API 密钥</h3>
            <p className="text-sm text-muted-foreground">活跃密钥</p>
          </div>
        </Link>

        <Link href="/preauth-keys" className="group">
          <div className="bg-card rounded-2xl p-6 border border-border hover:border-primary/50 transition-all hover:shadow-lg hover:shadow-primary/5">
            <div className="flex items-center justify-between mb-4">
              <div className="w-12 h-12 rounded-xl bg-gradient-to-br from-emerald-500 to-green-500 flex items-center justify-center">
                <svg className="w-6 h-6 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                </svg>
              </div>
              <span className="text-3xl font-bold text-foreground">0</span>
            </div>
            <h3 className="font-medium text-foreground">预认证密钥</h3>
            <p className="text-sm text-muted-foreground">可用密钥</p>
          </div>
        </Link>
      </div>

      {/* 快捷操作 */}
      <div className="grid grid-cols-2 gap-6">
        <div className="bg-card rounded-2xl p-6 border border-border">
          <h2 className="text-lg font-semibold text-foreground mb-4">快速开始</h2>
          <div className="space-y-3">
            <div className="flex items-center gap-3 p-3 rounded-lg bg-muted/50">
              <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary font-bold">1</div>
              <div>
                <p className="font-medium text-foreground">创建用户</p>
                <p className="text-sm text-muted-foreground">添加命名空间来组织节点</p>
              </div>
            </div>
            <div className="flex items-center gap-3 p-3 rounded-lg bg-muted/50">
              <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary font-bold">2</div>
              <div>
                <p className="font-medium text-foreground">生成预认证密钥</p>
                <p className="text-sm text-muted-foreground">用于节点注册</p>
              </div>
            </div>
            <div className="flex items-center gap-3 p-3 rounded-lg bg-muted/50">
              <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary font-bold">3</div>
              <div>
                <p className="font-medium text-foreground">连接节点</p>
                <p className="text-sm text-muted-foreground">使用密钥注册设备</p>
              </div>
            </div>
          </div>
        </div>

        <div className="bg-card rounded-2xl p-6 border border-border">
          <h2 className="text-lg font-semibold text-foreground mb-4">系统状态</h2>
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">数据库</span>
              <span className="flex items-center gap-2 text-green-500">
                <div className="w-2 h-2 rounded-full bg-green-500"></div>
                正常
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">HTTP 服务</span>
              <span className="flex items-center gap-2 text-green-500">
                <div className="w-2 h-2 rounded-full bg-green-500"></div>
                运行中
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">gRPC 服务</span>
              <span className="flex items-center gap-2 text-green-500">
                <div className="w-2 h-2 rounded-full bg-green-500"></div>
                运行中
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}