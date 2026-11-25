-- AlterTable
ALTER TABLE "Product" ADD COLUMN "stripePriceId" TEXT;

-- CreateIndex
CREATE UNIQUE INDEX "Product_stripePriceId_key" ON "Product"("stripePriceId");
