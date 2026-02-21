'use client';

import { Resource } from '@/lib/types';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Database, Globe } from 'lucide-react';

interface ResourceInfoSectionProps {
  resource: Resource;
}

export function ResourceInfoSection({ resource }: ResourceInfoSectionProps) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="space-y-1">
            <CardTitle className="flex items-center gap-2">
              <Database className="h-5 w-5" />
              Resource Information
            </CardTitle>
            <CardDescription>
              Details about this protected resource
            </CardDescription>
          </div>
          <Badge variant="outline" className="bg-blue-50 text-blue-700">
            Resource
          </Badge>
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="grid gap-6 md:grid-cols-2">
          {/* Name */}
          <div>
            <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              Name
            </p>
            <p className="mt-2 text-lg font-medium">{resource.name}</p>
          </div>

          {/* Address */}
          <div>
            <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              Address
            </p>
            <p className="mt-2 flex items-center gap-2 font-mono text-sm">
              <Globe className="h-4 w-4 text-muted-foreground" />
              {resource.address}
            </p>
          </div>

          {/* Description */}
          <div className="md:col-span-2">
            <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              Description
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              {resource.description}
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
