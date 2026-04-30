/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo } from "react";
import { 
  BarChart, Bar, LineChart, Line, AreaChart, Area, PieChart, Pie, Cell,
  XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer 
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { BarChart3, LineChart as LineChartIcon, PieChart as PieChartIcon, Activity, Settings2 } from "lucide-react";

const COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#14b8a6', '#f97316'];

interface ChartBuilderProps {
  data?: any[];
  columns?: string[];
}

export function ChartBuilder({ data = [], columns = [] }: ChartBuilderProps) {
  const [chartType, setChartType] = useState<'bar' | 'line' | 'area' | 'pie'>('bar');
  const [xAxis, setXAxis] = useState<string>(columns[0] || "");
  const [yAxis, setYAxis] = useState<string>(columns[1] || columns[0] || "");

  // Auto-detect numeric columns for better Y-axis defaults
  const numericColumns = useMemo(() => {
    if (data.length === 0) return columns;
    return columns.filter(col => typeof data[0][col] === 'number' || !isNaN(Number(data[0][col])));
  }, [data, columns]);

  // Ensure data is parsed properly for numeric values
  const parsedData = useMemo(() => {
    return data.map(row => {
      const newRow = { ...row };
      if (yAxis && newRow[yAxis] !== null && newRow[yAxis] !== undefined) {
        newRow[yAxis] = Number(newRow[yAxis]);
      }
      return newRow;
    });
  }, [data, yAxis]);

  if (!data || data.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center p-8 text-[var(--muted-foreground)]">
        <Activity size={32} className="opacity-20 mb-2" />
        <p>No data available to visualize.</p>
      </div>
    );
  }

  const renderChart = () => {
    switch (chartType) {
      case 'line':
        return (
          <LineChart data={parsedData}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.1)" />
            <XAxis dataKey={xAxis} stroke="rgba(255,255,255,0.5)" fontSize={12} />
            <YAxis stroke="rgba(255,255,255,0.5)" fontSize={12} />
            <Tooltip 
              contentStyle={{ backgroundColor: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
            />
            <Legend />
            <Line type="monotone" dataKey={yAxis} stroke={COLORS[0]} strokeWidth={3} activeDot={{ r: 8 }} />
          </LineChart>
        );
      case 'area':
        return (
          <AreaChart data={parsedData}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.1)" />
            <XAxis dataKey={xAxis} stroke="rgba(255,255,255,0.5)" fontSize={12} />
            <YAxis stroke="rgba(255,255,255,0.5)" fontSize={12} />
            <Tooltip 
              contentStyle={{ backgroundColor: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
            />
            <Legend />
            <Area type="monotone" dataKey={yAxis} fill={COLORS[1]} stroke={COLORS[1]} fillOpacity={0.3} />
          </AreaChart>
        );
      case 'pie':
        return (
          <PieChart>
            <Tooltip 
              contentStyle={{ backgroundColor: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
            />
            <Legend />
            <Pie
              data={parsedData}
              cx="50%"
              cy="50%"
              labelLine={false}
              outerRadius={120}
              fill="#8884d8"
              dataKey={yAxis}
              nameKey={xAxis}
              label={({ name, percent }) => `${name} ${((percent || 0) * 100).toFixed(0)}%`}
            >
              {parsedData.map((_entry, index) => (
                <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
              ))}
            </Pie>
          </PieChart>
        );
      case 'bar':
      default:
        return (
          <BarChart data={parsedData}>
            <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.1)" />
            <XAxis dataKey={xAxis} stroke="rgba(255,255,255,0.5)" fontSize={12} />
            <YAxis stroke="rgba(255,255,255,0.5)" fontSize={12} />
            <Tooltip 
              contentStyle={{ backgroundColor: 'var(--card)', borderColor: 'var(--border)', color: 'var(--foreground)' }}
              cursor={{ fill: 'rgba(255,255,255,0.05)' }}
            />
            <Legend />
            <Bar dataKey={yAxis} fill={COLORS[0]} radius={[4, 4, 0, 0]} />
          </BarChart>
        );
    }
  };

  return (
    <Card className="border-[var(--border)] bg-[var(--card)]/50 h-full flex flex-col">
      <CardHeader className="py-3 border-b border-[var(--border)] bg-[var(--muted)]/20 flex flex-row items-center justify-between">
        <CardTitle className="text-sm font-bold flex items-center gap-2">
          <Activity size={16} className="text-blue-500" />
          Smart Visualization
        </CardTitle>
        <div className="flex items-center gap-2">
          <div className="flex bg-[var(--muted)] p-1 rounded-lg">
            <Button 
              variant={chartType === 'bar' ? 'primary' : 'ghost'} 
              size="sm" 
              className="h-7 px-2"
              onClick={() => setChartType('bar')}
            >
              <BarChart3 size={14} />
            </Button>
            <Button 
              variant={chartType === 'line' ? 'primary' : 'ghost'} 
              size="sm" 
              className="h-7 px-2"
              onClick={() => setChartType('line')}
            >
              <LineChartIcon size={14} />
            </Button>
            <Button 
              variant={chartType === 'area' ? 'primary' : 'ghost'} 
              size="sm" 
              className="h-7 px-2"
              onClick={() => setChartType('area')}
            >
              <Activity size={14} />
            </Button>
            <Button 
              variant={chartType === 'pie' ? 'primary' : 'ghost'} 
              size="sm" 
              className="h-7 px-2"
              onClick={() => setChartType('pie')}
            >
              <PieChartIcon size={14} />
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="p-4 flex-1 flex flex-col gap-4 min-h-[400px]">
        
        {/* Controls */}
        <div className="flex flex-wrap items-center gap-4 p-3 bg-[var(--muted)]/30 border border-[var(--border)] rounded-lg">
          <div className="flex items-center gap-2">
            <Settings2 size={14} className="text-[var(--muted-foreground)]" />
            <span className="text-xs font-medium uppercase text-[var(--muted-foreground)]">Axes</span>
          </div>
          
          <div className="flex items-center gap-2">
            <span className="text-xs">X-Axis:</span>
            <select 
              className="h-8 px-2 text-xs bg-[var(--background)] border border-[var(--border)] rounded-md"
              value={xAxis}
              onChange={(e) => setXAxis(e.target.value)}
            >
              {columns.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          </div>

          <div className="flex items-center gap-2">
            <span className="text-xs">Y-Axis (Value):</span>
            <select 
              className="h-8 px-2 text-xs bg-[var(--background)] border border-[var(--border)] rounded-md"
              value={yAxis}
              onChange={(e) => setYAxis(e.target.value)}
            >
              {numericColumns.map(c => <option key={c} value={c}>{c}</option>)}
              {columns.filter(c => !numericColumns.includes(c)).map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          </div>
        </div>

        {/* Chart Area */}
        <div className="flex-1 w-full h-full min-h-[300px]">
          <ResponsiveContainer width="100%" height="100%">
            {renderChart()}
          </ResponsiveContainer>
        </div>

      </CardContent>
    </Card>
  );
}
