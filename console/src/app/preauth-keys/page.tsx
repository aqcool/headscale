"use client";

import { useEffect, useState } from "react";
import { api, PreAuthKey } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";

export default function PreAuthKeysPage() {
  const [keys, setKeys] = useState<PreAuthKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newKey, setNewKey] = useState<PreAuthKey | null>(null);
  const [form, setForm] = useState({ reusable: false, ephemeral: false });

  useEffect(() => {
    loadKeys();
  }, []);

  async function loadKeys() {
    try {
      setLoading(true);
      const data = await api.listPreAuthKeys();
      setKeys(data.preAuthKeys || []);
      setError(null);
    } catch (e) {
      setError("加载预认证密钥失败");
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate() {
    try {
      const data = await api.createPreAuthKey(form);
      setNewKey(data.preAuthKey);
      setDialogOpen(false);
      loadKeys();
    } catch (e) {
      setError("创建预认证密钥失败");
    }
  }

  async function handleDelete(key: string) {
    if (!confirm("确定删除此预认证密钥？")) return;
    try {
      await api.deletePreAuthKey(key);
      loadKeys();
    } catch (e) {
      setError("删除预认证密钥失败");
    }
  }

  return (
    <div className="p-8">
      <div className="flex justify-between items-center mb-8">
        <div>
          <h1 className="text-3xl font-bold text-foreground">预认证密钥管理</h1>
          <p className="text-muted-foreground mt-2">管理用于节点注册的预认证密钥</p>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger render={<Button className="gap-2">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
            创建密钥
          </Button>} />
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle className="text-xl">创建预认证密钥</DialogTitle>
            </DialogHeader>
            <div className="space-y-4 pt-4">
              <div className="flex items-center gap-3 p-3 rounded-lg bg-muted/50">
                <Input
                  type="checkbox"
                  checked={form.reusable}
                  onChange={(e) => setForm({ ...form, reusable: e.target.checked })}
                  className="w-5 h-5"
                />
                <div>
                  <Label className="font-medium">可重复使用</Label>
                  <p className="text-sm text-muted-foreground">允许多个节点使用同一密钥注册</p>
                </div>
              </div>
              <div className="flex items-center gap-3 p-3 rounded-lg bg-muted/50">
                <Input
                  type="checkbox"
                  checked={form.ephemeral}
                  onChange={(e) => setForm({ ...form, ephemeral: e.target.checked })}
                  className="w-5 h-5"
                />
                <div>
                  <Label className="font-medium">临时节点</Label>
                  <p className="text-sm text-muted-foreground">节点离线后自动删除</p>
                </div>
              </div>
              <Button className="w-full mt-4" onClick={handleCreate}>创建</Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      {newKey && (
        <div className="mb-6 p-5 rounded-xl bg-gradient-to-r from-violet-500/10 to-purple-500/10 border border-violet-500/20">
          <div className="flex items-start justify-between">
            <div>
              <h3 className="font-semibold text-violet-400 flex items-center gap-2">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                预认证密钥已创建
              </h3>
              <p className="text-sm text-muted-foreground mt-1">请妥善保存，用于节点注册</p>
              <code className="block mt-3 p-3 rounded-lg bg-black/50 font-mono text-sm text-violet-300">{newKey.key}</code>
            </div>
            <Button size="sm" variant="ghost" onClick={() => setNewKey(null)}>关闭</Button>
          </div>
        </div>
      )}

      {error && (
        <div className="mb-6 p-4 rounded-lg bg-destructive/10 text-destructive border border-destructive/20">
          {error}
        </div>
      )}

      <div className="bg-card rounded-2xl border border-border overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-muted-foreground">
            <svg className="w-8 h-8 mx-auto mb-4 animate-spin text-primary" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
            </svg>
            加载中...
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/50">
                <TableHead className="font-semibold">ID</TableHead>
                <TableHead className="font-semibold">密钥</TableHead>
                <TableHead className="font-semibold">可复用</TableHead>
                <TableHead className="font-semibold">临时</TableHead>
                <TableHead className="font-semibold">已使用</TableHead>
                <TableHead className="font-semibold">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center py-12 text-muted-foreground">
                    <svg className="w-12 h-12 mx-auto mb-4 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                    </svg>
                    暂无预认证密钥
                  </TableCell>
                </TableRow>
              ) : (
                keys.map((key) => (
                  <TableRow key={key.id} className="hover:bg-muted/30 transition-colors">
                    <TableCell className="font-mono text-sm">{key.id}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="font-mono">{key.key.slice(0, 12)}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={key.reusable ? "default" : "secondary"}>
                        {key.reusable ? "是" : "否"}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={key.ephemeral ? "default" : "secondary"}>
                        {key.ephemeral ? "是" : "否"}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={key.used ? "destructive" : "outline"}>
                        {key.used ? "已使用" : "未使用"}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={() => handleDelete(key.key)}
                      >
                        删除
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        )}
      </div>
    </div>
  );
}