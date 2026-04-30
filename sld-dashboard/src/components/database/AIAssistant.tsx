import { useState, useRef, useEffect } from "react";
import { Sparkles, Send, Database, Terminal, Check, Copy, User } from "lucide-react";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useTables } from "@/hooks/use-database";
import Editor from "@monaco-editor/react";

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  sql?: string;
}

export function AIAssistant({ database }: { database: string }) {
  const { data: tables = [] } = useTables(database);
  const [messages, setMessages] = useState<Message[]>([
    {
      id: "1",
      role: 'assistant',
      content: `Hello! I'm your AI Data Assistant for **${database}**. I've analyzed your ${tables.length} tables. Ask me anything about your data!`
    }
  ]);
  const [input, setInput] = useState("");
  const [isTyping, setIsTyping] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages, isTyping]);

  const handleSend = async () => {
    if (!input.trim()) return;

    const userMsg: Message = { id: Date.now().toString(), role: 'user', content: input };
    setMessages(prev => [...prev, userMsg]);
    setInput("");
    setIsTyping(true);

    // Simulate AI delay
    setTimeout(() => {
      let responseSql = "";
      let responseText = "Here is the query you requested based on your schema:";
      
      const query = userMsg.content.toLowerCase();
      
      if (query.includes("top") || query.includes("highest")) {
        responseSql = `SELECT * FROM ${tables[0]?.name || 'your_table'}\nORDER BY id DESC\nLIMIT 10;`;
      } else if (query.includes("count") || query.includes("how many")) {
        responseSql = `SELECT COUNT(*) as total FROM ${tables[0]?.name || 'your_table'};`;
      } else if (query.includes("join") || query.includes("together")) {
        responseSql = `SELECT a.*, b.*\nFROM ${tables[0]?.name || 'table_a'} a\nJOIN ${tables[1]?.name || 'table_b'} b ON a.id = b.${tables[0]?.name || 'table_a'}_id;`;
      } else {
        responseSql = `SELECT * FROM ${tables[0]?.name || 'your_table'} LIMIT 100;`;
        responseText = "I've generated a basic query to get you started. Could you be more specific?";
      }

      setMessages(prev => [...prev, {
        id: (Date.now() + 1).toString(),
        role: 'assistant',
        content: responseText,
        sql: responseSql
      }]);
      setIsTyping(false);
    }, 1500);
  };

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 2000);
  };

  return (
    <Card className="flex flex-col h-[calc(100vh-16rem)] border-[var(--border)] overflow-hidden shadow-xl ring-1 ring-blue-500/20">
      <CardHeader className="border-b border-[var(--border)] bg-gradient-to-r from-blue-500/10 to-transparent py-4">
        <CardTitle className="text-lg font-bold flex items-center gap-2">
          <Sparkles className="text-blue-500" />
          Data Intelligence AI
        </CardTitle>
        <p className="text-xs text-[var(--muted-foreground)]">Natural Language to SQL Engine</p>
      </CardHeader>
      
      <CardContent className="flex-1 overflow-y-auto p-4 space-y-6 bg-[var(--background)]">
        {messages.map((msg) => (
          <div key={msg.id} className={`flex gap-4 ${msg.role === 'user' ? 'flex-row-reverse' : ''}`}>
            <div className={`w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0 ${msg.role === 'user' ? 'bg-blue-500' : 'bg-[var(--primary)]/20 text-[var(--primary)]'}`}>
              {msg.role === 'user' ? <User size={16} /> : <Sparkles size={16} />}
            </div>
            <div className={`flex-1 max-w-[80%] ${msg.role === 'user' ? 'text-right' : 'text-left'}`}>
              <div className={`inline-block p-3 rounded-2xl text-sm ${msg.role === 'user' ? 'bg-blue-500 text-white rounded-tr-none' : 'bg-[var(--muted)]/50 border border-[var(--border)] rounded-tl-none'}`}>
                {msg.content}
              </div>
              
              {msg.sql && (
                <div className="mt-3 relative rounded-xl overflow-hidden border border-blue-500/30 shadow-lg">
                  <div className="absolute top-2 right-2 z-10 flex gap-2">
                    <Button 
                      size="icon" 
                      variant="secondary" 
                      className="h-7 w-7 bg-black/50 hover:bg-black/80 text-white"
                      onClick={() => copyToClipboard(msg.sql!, msg.id)}
                    >
                      {copiedId === msg.id ? <Check size={14} className="text-green-400" /> : <Copy size={14} />}
                    </Button>
                  </div>
                  <div className="bg-blue-500/10 px-3 py-1.5 text-xs font-medium text-blue-400 border-b border-blue-500/20 flex items-center gap-2">
                    <Terminal size={12} /> Generated SQL
                  </div>
                  <Editor
                    height="120px"
                    defaultLanguage="sql"
                    theme="vs-dark"
                    value={msg.sql}
                    options={{
                      readOnly: true,
                      minimap: { enabled: false },
                      scrollBeyondLastLine: false,
                      fontSize: 13,
                      padding: { top: 12, bottom: 12 }
                    }}
                  />
                </div>
              )}
            </div>
          </div>
        ))}
        {isTyping && (
          <div className="flex gap-4">
            <div className="w-8 h-8 rounded-full bg-[var(--primary)]/20 text-[var(--primary)] flex items-center justify-center">
              <Sparkles size={16} className="animate-pulse" />
            </div>
            <div className="p-3 bg-[var(--muted)]/50 rounded-2xl rounded-tl-none border border-[var(--border)] flex gap-1 items-center">
              <div className="w-2 h-2 rounded-full bg-[var(--primary)]/50 animate-bounce" style={{ animationDelay: '0ms' }} />
              <div className="w-2 h-2 rounded-full bg-[var(--primary)]/50 animate-bounce" style={{ animationDelay: '150ms' }} />
              <div className="w-2 h-2 rounded-full bg-[var(--primary)]/50 animate-bounce" style={{ animationDelay: '300ms' }} />
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </CardContent>

      <div className="p-4 border-t border-[var(--border)] bg-[var(--muted)]/20">
        <form 
          className="relative flex items-center"
          onSubmit={(e) => { e.preventDefault(); handleSend(); }}
        >
          <Input 
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Ask a question about your data... (e.g., 'Show me the top 10 rows')"
            className="pr-12 h-12 bg-[var(--background)] border-blue-500/30 focus-visible:ring-blue-500/50 shadow-inner rounded-full"
            disabled={isTyping}
          />
          <Button 
            type="submit" 
            size="icon" 
            className="absolute right-1.5 h-9 w-9 rounded-full bg-blue-500 hover:bg-blue-600 shadow-md"
            disabled={!input.trim() || isTyping}
          >
            <Send size={16} />
          </Button>
        </form>
        <div className="flex items-center justify-center gap-4 mt-3 text-[10px] text-[var(--muted-foreground)]">
           <span className="flex items-center gap-1"><Database size={10} /> Schema-Aware</span>
           <span className="flex items-center gap-1"><Sparkles size={10} /> Auto-Joins</span>
        </div>
      </div>
    </Card>
  );
}
