"use client";

import { useEffect, useState } from "react";
import { api, ApiKey } from "@/lib/api";
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

export default function ApiKeysPage() {
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [newKey, setNewKey] = useState<string | null>(null);

  useEffect(() => {
    loadKeys();
  }, []);

  async function loadKeys() {
    try {
      setLoading(true);
      const data = await api.listApiKeys();
      setKeys(data.apiKeys || []);
      setError(null);
    } catch (e) {
      setError("加载 API 密钥失败");
    } finally {
      setLoading(false);
    }
  }

  async function handleCreate() {
    try {
      const data = await api.createApiKey();
      setNewKey(data.apiKey);
      loadKeys();
    } catch (e) {
      setError("创建 API 密钥失败");
    }
  }

  async function handleDelete(prefix: string) {
    if (!confirm("确定删除此 API 密钥？")) return;
    try {
      await api.deleteApiKey(prefix);
      loadKeys();
    } catch (e) {
      setError("删除 API 密钥失败");
    }
  }

  return (
    <div className="p-8">
      <div className="flex justify-between items-center mb-8">
        <div>
          <h1 className="text-3xl font-bold text-foreground">API 密钥管理</h1>
          <p className="text-muted-foreground mt-2">管理用于 API 认证的密钥</p>
        </div>
        <Button onClick={handleCreate} className="gap-2">
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          创建密钥
        </Button>
      </div>

      {newKey && (
        <div className="mb-6 p-5 rounded-xl bg-gradient-to-r from-emerald-500/10 to-green-500/10 border border-emerald-500/20">
          <div className="flex items-start justify-between">
            <div>
              <h3 className="font-semibold text-emerald-500 flex items-center gap-2">
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                API 密钥已创建
              </h3>
              <p className="text-sm text-muted-foreground mt-1">请妥善保存，此密钥仅显示一次</p>
              <code className="block mt-3 p-3 rounded-lg bg-black/50 font-mono text-sm text-emerald-400">{newKey}</code>
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
                <TableHead className="font-semibold">前缀</TableHead>
                <TableHead className="font-semibold">创建时间</TableHead>
                <TableHead className="font-semibold">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="text-center py-12 text-muted-foreground">
                    <svg className="w-12 h-12 mx-auto mb-4 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                    </svg>
                    暂无 API 密钥
                  </TableCell>
                </TableRow>
              ) : (
                keys.map((key) => (
                  <TableRow key={key.id} className="hover:bg-muted/30 transition-colors">
                    <TableCell className="font-mono text-sm">{key.id}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="font-mono">{key.prefix}</Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">{key.createdAt || "-"}</TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive hover:bg-destructive/10"
                        onClick={() => handleDelete(key.prefix)}
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