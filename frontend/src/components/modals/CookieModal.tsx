'use client';

import React, { useState, useEffect } from 'react';
import { Dialog, DialogHeader } from '@astryxdesign/core/Dialog';
import { Button } from '@astryxdesign/core/Button';
import { TextArea } from '@astryxdesign/core/TextArea';
import { Banner } from '@astryxdesign/core/Banner';
import { getSavedCookies, setSavedCookies } from '@/lib/api';
import { Save, Trash2, Check, Cookie } from 'lucide-react';

interface CookieModalProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CookieModal({ isOpen, onOpenChange }: CookieModalProps) {
  const [cookieText, setCookieText] = useState('');
  const [savedFeedback, setSavedFeedback] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setCookieText(getSavedCookies());
      setSavedFeedback(false);
    }
  }, [isOpen]);

  const handleSave = () => {
    setSavedCookies(cookieText);
    setSavedFeedback(true);
    setTimeout(() => {
      setSavedFeedback(false);
      onOpenChange(false);
    }, 650);
  };

  const handleClear = () => {
    setCookieText('');
    setSavedCookies('');
    setSavedFeedback(false);
  };

  return (
    <Dialog
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      width={600}
      maxHeight="82dvh"
      purpose="form"
    >
      <div className="flex flex-col">
        <DialogHeader
          title="Cookie Manager"
          subtitle="Netscape cookies.txt for age-gated or member-only downloads"
          onOpenChange={onOpenChange}
          startContent={
            <div className="w-8 h-8 rounded-lg bg-[var(--color-warning-muted)] flex items-center justify-center text-[var(--color-text-warning)]">
              <Cookie className="w-4 h-4" />
            </div>
          }
        />

        <div className="p-4 space-y-3">
          <Banner
            status="info"
            title="Stored locally in this browser"
            description="Your cookies are sent directly with download and channel scan requests, and can be cleared at any time."
          />

          <TextArea
            label="Netscape cookies.txt content"
            value={cookieText}
            onChange={(val) => setCookieText(val)}
            rows={9}
            placeholder="# Netscape HTTP Cookie File&#10;.youtube.com&#9;TRUE&#9;/&#9;TRUE&#9;1799999999&#9;SID&#9;..."
          />
        </div>

        <div className="px-4 py-3 border-t border-[var(--color-border)] bg-[var(--color-background-surface)] flex items-center justify-between gap-2">
          <Button
            label="Clear Saved Cookies"
            variant="ghost"
            size="sm"
            icon={<Trash2 className="w-3.5 h-3.5" />}
            onClick={handleClear}
          />
          <div className="flex items-center gap-2">
            <Button
              label="Cancel"
              variant="secondary"
              size="sm"
              onClick={() => onOpenChange(false)}
            />
            <Button
              label={savedFeedback ? 'Saved' : 'Save Cookies'}
              variant="primary"
              size="sm"
              icon={
                savedFeedback ? (
                  <Check className="w-3.5 h-3.5" />
                ) : (
                  <Save className="w-3.5 h-3.5" />
                )
              }
              onClick={handleSave}
            />
          </div>
        </div>
      </div>
    </Dialog>
  );
}
